package at

import (
	"context"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func nullReg() SeriesRegistration {
	return SeriesRegistration{
		SeriesID: "S2026", DocType: domain.FT, SeriesType: "N", InitialSeq: 1, ExpectedStartDate: atT0,
	}
}

// TestNullClockInjectable pins that NullClient timestamps are deterministic
// when a clock is injected — RegistrationDate feeds Series.RegisterWithAT,
// which the domain's date-before-registration guard compares against fixture
// dates; a wall-clock fake makes such tests fail nondeterministically.
func TestNullClockInjectable(t *testing.T) {
	c := NewNullClient()
	fixed := atT0
	c.Now = func() time.Time { return fixed }
	res, err := c.RegisterSeries(context.Background(), nullReg())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !res.RegistrationDate.Equal(fixed) {
		t.Fatalf("RegistrationDate = %v, want injected clock %v", res.RegistrationDate, fixed)
	}
}

func TestNullRegisterIsDeterministic(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r1, err := c.RegisterSeries(ctx, nullReg())
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := domain.ValidateATCode(r1.ValidationCode); err != nil {
		t.Fatalf("generated code invalid: %v", err)
	}
	r2, err := c.RegisterSeries(ctx, nullReg())
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if r1.ValidationCode != r2.ValidationCode {
		t.Errorf("codes differ on repeat registration: %q vs %q", r1.ValidationCode, r2.ValidationCode)
	}
}

func TestNullCodesUniquePerSeries(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r1, _ := c.RegisterSeries(ctx, nullReg())
	other := nullReg()
	other.DocType = domain.NC
	r2, err := c.RegisterSeries(ctx, other)
	if err != nil {
		t.Fatalf("register NC: %v", err)
	}
	if r1.ValidationCode == r2.ValidationCode {
		t.Error("different series must get different codes")
	}
}

func TestNullRegisterValidatesSeriesID(t *testing.T) {
	c := NewNullClient()
	bad := nullReg()
	bad.SeriesID = "A/B" // "/" breaks the SAF-T DocumentNumber pattern
	if _, err := c.RegisterSeries(context.Background(), bad); err == nil {
		t.Fatal("want validation error for series id containing /")
	}
	// "AT"-prefixed ids are no longer rejected locally — the claimed reserved
	// prefix is unconfirmed (docs/series-rules.yaml series-identifier-no-reserved-prefix).
	ok := nullReg()
	ok.SeriesID = "AT2026"
	if _, err := c.RegisterSeries(context.Background(), ok); err != nil {
		t.Fatalf("AT-prefixed series id rejected: %v", err)
	}
}

func TestNullLifecycle(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r, _ := c.RegisterSeries(ctx, nullReg())

	st, err := c.GetSeriesStatus(ctx, "S2026", domain.FT)
	if err != nil || st.Status != domain.SeriesActive {
		t.Fatalf("status = %+v, %v", st, err)
	}

	if err := c.FinalizeSeries(ctx, SeriesFinalization{
		SeriesID: "S2026", DocType: domain.FT, ATCode: r.ValidationCode, LastSeq: 3,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	st, _ = c.GetSeriesStatus(ctx, "S2026", domain.FT)
	if st.Status != domain.SeriesFinalized || st.LastSeq != 3 {
		t.Errorf("after finalize: %+v", st)
	}
}

func TestNullCancel(t *testing.T) {
	c := NewNullClient()
	ctx := context.Background()

	r, _ := c.RegisterSeries(ctx, nullReg())
	if err := c.CancelSeries(ctx, SeriesCancellation{
		SeriesID: "S2026", DocType: domain.FT, ATCode: r.ValidationCode, Reason: CancelReasonError,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	st, _ := c.GetSeriesStatus(ctx, "S2026", domain.FT)
	if st.Status != domain.SeriesCancelled {
		t.Errorf("after cancel: %+v", st)
	}
}

func TestNullUnknownSeries(t *testing.T) {
	c := NewNullClient()
	if _, err := c.GetSeriesStatus(context.Background(), "NOPE", domain.FT); err == nil {
		t.Fatal("want error for unknown series")
	}
	if err := c.FinalizeSeries(context.Background(), SeriesFinalization{
		SeriesID: "NOPE", DocType: domain.FT, ATCode: "BCDFGH37",
	}); err == nil {
		t.Fatal("want error finalizing unknown series")
	}
}

func TestNullCommunicateTransport(t *testing.T) {
	c := NewNullClient()
	res, err := c.CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if res.ATDocCodeID == "" {
		t.Fatal("fake must return a non-empty ATDocCodeID")
	}
	res2, _ := c.CommunicateTransport(context.Background(), testCompany(t), testMovement(t))
	if res.ATDocCodeID != res2.ATDocCodeID {
		t.Error("fake ATDocCodeID must be deterministic per document number")
	}
}

func TestNullCommunicateInvoice(t *testing.T) {
	c := NewNullClient()
	res, err := c.CommunicateInvoice(context.Background(), testCompany(t), testInvoice(t))
	if err != nil {
		t.Fatalf("invoice: %v", err)
	}
	if res.Code != 0 {
		t.Errorf("Code = %d, want 0", res.Code)
	}
}

func TestNullClient_CancelledTransportGetsNoATDocCode(t *testing.T) {
	c := NewNullClient()
	mv := testMovement(t)
	mv.Status = domain.StatusCancelled
	res, err := c.CommunicateTransport(context.Background(), testCompany(t), mv)
	if err != nil {
		t.Fatal(err)
	}
	if res.ATDocCodeID != "" {
		t.Fatalf("cancelled movement got ATDocCodeID %q; the real sgdtws returns none", res.ATDocCodeID)
	}
}
