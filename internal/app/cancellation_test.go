package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func issueOne(t *testing.T, svc *app.Services) domain.SalesInvoice {
	t.Helper()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	doc, err := svc.Invoicing.IssueSalesInvoice(
		context.Background(), testTenantID, app.IssueSalesInvoiceRequest{
			Draft: draft, SeriesID: "FT2026", SourceID: "src-1", Idem: app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"},
		},
	)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return doc
}

func TestCancelDocument_HappyPath(t *testing.T) {
	svc, store := newFixture()
	doc := issueOne(t, svc)

	cancelled, err := svc.Invoicing.CancelDocument(context.Background(), testTenantID, doc.Number, "engano de faturação")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != domain.StatusCancelled {
		t.Fatalf("status = %q, want %q", cancelled.Status, domain.StatusCancelled)
	}
	if cancelled.Reason != "engano de faturação" {
		t.Fatalf("reason = %q, want %q", cancelled.Reason, "engano de faturação")
	}
	stored, ok := store.GetSalesInvoice(testTenantID, doc.Number)
	if !ok || stored.Status != domain.StatusCancelled {
		t.Fatalf("stored status = %q (ok=%v), want cancelled", stored.Status, ok)
	}
}

func TestCancelDocument_NotFound(t *testing.T) {
	svc, _ := newFixture()
	num := mustVal(domain.NewDocNumber(domain.FT, "FT2026", 99)) // never issued
	_, err := svc.Invoicing.CancelDocument(context.Background(), testTenantID, num, "x")
	if app.KindOf(err) != app.KindNotFound {
		t.Fatalf("kind = %v, want KindNotFound", app.KindOf(err))
	}
}

func TestCancelDocument_AlreadyCancelled(t *testing.T) {
	svc, _ := newFixture()
	doc := issueOne(t, svc)

	if _, err := svc.Invoicing.CancelDocument(context.Background(), testTenantID, doc.Number, "first"); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	// Second cancel proves the first persisted (the reloaded doc is already "A").
	_, err := svc.Invoicing.CancelDocument(context.Background(), testTenantID, doc.Number, "second")
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict (already cancelled)", app.KindOf(err))
	}
}

// TestCancelDocument_BlockedByLiveNC tests V1: annulling a document that has a
// live NC referencing it is rejected with KindConflict / ErrHasLiveRectifyingNote,
// and the document's Status stays N (no mutation on rejection).
func TestCancelDocument_BlockedByLiveNC(t *testing.T) {
	now := testNow()
	ncSeries := activeSeries("NC2026", domain.NC, now)
	svc, store := newFixtureSeries(now, activeFTSeries(now), ncSeries)

	// Issue the FT.
	ft := issueOne(t, svc)

	// Issue an NC referencing the FT.
	ncD := ncDraft(ncSeries, now, ft)
	_, err := svc.Invoicing.IssueSalesInvoice(
		context.Background(), testTenantID, app.IssueSalesInvoiceRequest{
			Draft: ncD, SeriesID: "NC2026", SourceID: "src-nc",
			Idem: app.IdempotencyKey{Key: "nc-k1", Fingerprint: "nc-fp1"},
		},
	)
	if err != nil {
		t.Fatalf("issue NC: %v", err)
	}

	// Attempt to cancel the FT — must be blocked.
	_, cerr := svc.Invoicing.CancelDocument(context.Background(), testTenantID, ft.Number, "annul test")
	if app.KindOf(cerr) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", app.KindOf(cerr))
	}
	if !errors.Is(cerr, domain.ErrHasLiveRectifyingNote) {
		t.Fatalf("err = %v, want ErrHasLiveRectifyingNote", cerr)
	}

	// Verify FT is still Status N (not mutated by the rejected cancel).
	stored, ok := store.GetSalesInvoice(testTenantID, ft.Number)
	if !ok {
		t.Fatal("FT must still exist in store")
	}
	if stored.Status != domain.StatusNormal {
		t.Fatalf("FT status = %q after blocked cancel, want %q (N)", stored.Status, domain.StatusNormal)
	}
}

// TestCancelDocument_AllowedAfterNCCancelled tests V1: once the NC is cancelled,
// the underlying FT can be cancelled (succeeds, FT becomes Status A).
func TestCancelDocument_AllowedAfterNCCancelled(t *testing.T) {
	now := testNow()
	ncSeries := activeSeries("NC2026", domain.NC, now)
	svc, _ := newFixtureSeries(now, activeFTSeries(now), ncSeries)

	// Issue FT, then NC referencing it.
	ft := issueOne(t, svc)
	ncD := ncDraft(ncSeries, now, ft)
	nc, err := svc.Invoicing.IssueSalesInvoice(
		context.Background(), testTenantID, app.IssueSalesInvoiceRequest{
			Draft: ncD, SeriesID: "NC2026", SourceID: "src-nc",
			Idem: app.IdempotencyKey{Key: "nc-k2", Fingerprint: "nc-fp2"},
		},
	)
	if err != nil {
		t.Fatalf("issue NC: %v", err)
	}

	// Cancel the NC first.
	if _, cerr := svc.Invoicing.CancelDocument(context.Background(), testTenantID, nc.Number, "anular NC"); cerr != nil {
		t.Fatalf("cancel NC: %v", cerr)
	}

	// Now cancel the FT — must succeed.
	cancelled, cerr := svc.Invoicing.CancelDocument(context.Background(), testTenantID, ft.Number, "anular FT")
	if cerr != nil {
		t.Fatalf("cancel FT after NC cancelled: %v", cerr)
	}
	if cancelled.Status != domain.StatusCancelled {
		t.Fatalf("FT status = %q, want StatusCancelled", cancelled.Status)
	}
}

// TestCancelDocument_NoRectifierAllowed is a regression guard: a document with
// no live rectifying notes cancels normally (the V1 guard is not over-broad).
func TestCancelDocument_NoRectifierAllowed(t *testing.T) {
	svc, _ := newFixture()
	doc := issueOne(t, svc)

	cancelled, err := svc.Invoicing.CancelDocument(context.Background(), testTenantID, doc.Number, "sem notas")
	if err != nil {
		t.Fatalf("cancel with no rectifier: %v", err)
	}
	if cancelled.Status != domain.StatusCancelled {
		t.Fatalf("status = %q, want StatusCancelled", cancelled.Status)
	}
}

func TestCancelDocument_DeadlinePassed(t *testing.T) {
	svc, store := newFixture() // issue clock = testNow() = 2026-05-22
	doc := issueOne(t, svc)

	// A service whose clock is past the cancellation deadline for a May-22
	// invoice (day 5 of the following month → 2026-06-05 23:59:59 Lisbon).
	late := app.New(app.Deps{
		Tenants:  oneTenant(testTenant()),
		UoW:      store,
		Clock:    fixedClock{t: time.Date(2026, 7, 1, 9, 0, 0, 0, testLisbon())},
		Signer:   stubSigner{},
		Software: testSoftware(),
	})
	_, err := late.Invoicing.CancelDocument(context.Background(), testTenantID, doc.Number, "tarde demais")
	if app.KindOf(err) != app.KindConflict {
		t.Fatalf("kind = %v, want KindConflict", app.KindOf(err))
	}
	if !errors.Is(err, domain.ErrCancellationDeadlinePassed) {
		t.Fatalf("err = %v, want ErrCancellationDeadlinePassed", err)
	}
}
