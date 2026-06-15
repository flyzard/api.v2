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
		context.Background(), testTenantID, draft, "FT2026", "src-1",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"},
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
