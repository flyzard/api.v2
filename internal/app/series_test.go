package app_test

import (
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/adapter/at"
	"github.com/flyzard/invoicing.v2/internal/adapter/memstore"
	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

// newSeriesFixture wires the services with the null AT client and NO seeded
// series (tests create their own). The returned client is the one the factory
// hands out, so register/finalize/consult share AT-side state.
func newSeriesFixture() (*app.Services, *memstore.Store, *at.NullClient) {
	now := testNow()
	store := memstore.New()
	client := at.NewNullClient()
	client.Now = testNow // deterministic AT RegistrationDate, aligned with the test clock
	svc := app.New(app.Deps{
		Tenants:   oneTenant(testTenant()),
		UoW:       store,
		Queue:     store,
		Clock:     fixedClock{t: now},
		Signer:    stubSigner{},
		ATClients: nullATFactory{c: client},
		Software:  testSoftware(),
	})
	return svc, store, client
}

func TestCreateSeries_StoresUnregistered(t *testing.T) {
	svc, store, _ := newSeriesFixture()
	ser, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ser.CanIssue() {
		t.Fatal("a freshly created series must not be issuable until registered")
	}
	if _, ok := store.GetSeries(testTenantID, "FT2026", domain.FT); !ok {
		t.Fatal("series not persisted")
	}
}

func TestCreateSeries_DuplicateConflict(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	if _, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false)
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", app.KindOf(err))
	}
}

func TestCreateSeries_InvalidID(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	_, err := svc.Series.CreateSeries(context.Background(), testTenantID, "bad id/with space", domain.FT, false)
	if app.KindOf(err) != app.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", app.KindOf(err))
	}
}

func TestRegisterSeries_ActivatesWithATCode(t *testing.T) {
	svc, store, _ := newSeriesFixture()
	if _, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	ser, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "FT2026", domain.FT)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !ser.CanIssue() {
		t.Fatalf("registered series must be issuable; status=%s code=%q", ser.ATStatus, ser.ATCode)
	}
	stored, _ := store.GetSeries(testTenantID, "FT2026", domain.FT)
	if stored.ATCode == "" || !stored.CanIssue() {
		t.Fatalf("persisted series not active: %+v", stored)
	}
}

func TestRegisterSeries_AlreadyRegisteredConflict(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	if _, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "FT2026", domain.FT); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "FT2026", domain.FT)
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict (already registered)", app.KindOf(err))
	}
}

func TestRegisterSeries_NotFound(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	_, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "NOPE2026", domain.FT)
	if app.KindOf(err) != app.KindNotFound {
		t.Fatalf("kind = %v, want KindNotFound", app.KindOf(err))
	}
}

// registerAndIssue creates + registers "FT2026" and issues one invoice on it,
// so the series has LastNum >= 1.
func registerAndIssue(t *testing.T, svc *app.Services) {
	t.Helper()
	if _, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "FT2026", domain.FT); err != nil {
		t.Fatalf("register: %v", err)
	}
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	if _, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, draft, "FT2026", "src-1",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}); err != nil {
		t.Fatalf("issue: %v", err)
	}
}

func TestFinalizeSeries_AfterIssuing(t *testing.T) {
	svc, store, _ := newSeriesFixture()
	registerAndIssue(t, svc)

	ser, err := svc.Series.FinalizeSeries(context.Background(), testTenantID, "FT2026", domain.FT, "fim do exercício")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if ser.ATStatus != domain.SeriesFinalized {
		t.Fatalf("status = %s, want finalized", ser.ATStatus)
	}
	stored, _ := store.GetSeries(testTenantID, "FT2026", domain.FT)
	if stored.ATStatus != domain.SeriesFinalized {
		t.Fatalf("persisted status = %s, want finalized", stored.ATStatus)
	}
}

func TestCancelSeries_NeverIssued(t *testing.T) {
	svc, store, _ := newSeriesFixture()
	if _, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "FT2026", domain.FT); err != nil {
		t.Fatalf("register: %v", err)
	}
	ser, err := svc.Series.CancelSeries(context.Background(), testTenantID, "FT2026", domain.FT)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if ser.ATStatus != domain.SeriesCancelled {
		t.Fatalf("status = %s, want cancelled", ser.ATStatus)
	}
	stored, _ := store.GetSeries(testTenantID, "FT2026", domain.FT)
	if stored.ATStatus != domain.SeriesCancelled {
		t.Fatalf("persisted status = %s, want cancelled", stored.ATStatus)
	}
}

func TestCancelSeries_WithDocumentsConflict(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	registerAndIssue(t, svc) // LastNum >= 1, so anularSerie is illegal

	_, err := svc.Series.CancelSeries(context.Background(), testTenantID, "FT2026", domain.FT)
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict (series has documents)", app.KindOf(err))
	}
}

func TestConsultSeries_ReturnsATStatus(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	if _, err := svc.Series.CreateSeries(context.Background(), testTenantID, "FT2026", domain.FT, false); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Series.RegisterSeries(context.Background(), testTenantID, "FT2026", domain.FT); err != nil {
		t.Fatalf("register: %v", err)
	}
	st, err := svc.Series.ConsultSeries(context.Background(), testTenantID, "FT2026", domain.FT)
	if err != nil {
		t.Fatalf("consult: %v", err)
	}
	if st.Status != domain.SeriesActive || st.ValidationCode == "" {
		t.Fatalf("status = %+v, want active with a validation code", st)
	}
}

func TestConsultSeries_UnknownAtAT(t *testing.T) {
	svc, _, _ := newSeriesFixture()
	// Never registered with AT → the null client reports it unknown.
	_, err := svc.Series.ConsultSeries(context.Background(), testTenantID, "FT2026", domain.FT)
	if app.KindOf(err) != app.KindAT {
		t.Fatalf("kind = %v, want KindAT (series unknown at AT)", app.KindOf(err))
	}
}
