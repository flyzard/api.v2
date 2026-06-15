package app_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func issueQ(t *testing.T, svc *app.Services) domain.DocNumber {
	t.Helper()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	doc, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, draft, "FT2026", "src-1",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return doc.Number
}

func TestGetDocument_JoinsCommStatus(t *testing.T) {
	svc, _ := newCommFixture()
	num := issueQ(t, svc)

	view, err := svc.Query.GetDocument(context.Background(), testTenantID, num)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if view.Invoice.Number != num {
		t.Fatalf("got %s, want %s", view.Invoice.Number.Format(), num.Format())
	}
	if view.Comm == nil || view.Comm.Status != app.TaskPending {
		t.Fatalf("comm = %+v, want a pending task (CommRealtime enqueues)", view.Comm)
	}
}

func TestGetDocument_NotFound(t *testing.T) {
	svc, _ := newCommFixture()
	_, err := svc.Query.GetDocument(context.Background(), testTenantID,
		mustVal(domain.NewDocNumber(domain.FT, "FT2026", 99)))
	if app.KindOf(err) != app.KindNotFound {
		t.Fatalf("kind = %v, want KindNotFound", app.KindOf(err))
	}
}

func TestRenderPDF_ProducesPDF(t *testing.T) {
	svc, _ := newCommFixture()
	num := issueQ(t, svc)

	out, err := svc.Query.RenderPDF(context.Background(), testTenantID, num)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output is not a PDF (first bytes: %q)", out[:min(8, len(out))])
	}
}

func TestListSales_ReturnsInPeriod(t *testing.T) {
	svc, _ := newCommFixture()
	issueQ(t, svc)
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	list, err := svc.Query.ListSales(context.Background(), testTenantID,
		time.Date(2026, 5, 1, 0, 0, 0, 0, lisbon),
		time.Date(2026, 5, 31, 23, 59, 59, 0, lisbon))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d, want 1", len(list))
	}
}

func TestCommStatus_NotFound(t *testing.T) {
	svc, _ := newCommFixture()
	_, err := svc.Query.CommStatus(context.Background(), testTenantID,
		mustVal(domain.NewDocNumber(domain.FT, "FT2026", 99)))
	if app.KindOf(err) != app.KindNotFound {
		t.Fatalf("kind = %v, want KindNotFound", app.KindOf(err))
	}
}
