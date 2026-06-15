package app_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/app"
	"github.com/flyzard/invoicing.v2/internal/domain"
)

func TestIssueWorkDocument_HappyPath(t *testing.T) {
	now := testNow()
	svc, store := newFixtureSeries(now, activeSeries("NE2026", domain.NE, now))
	draft := workDraft(activeSeries("NE2026", domain.NE, now), now)

	doc, err := svc.Invoicing.IssueWorkDocument(
		context.Background(), testTenantID, draft, "NE2026", "src-1",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"},
	)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if doc.Number.Seq != 1 {
		t.Fatalf("seq = %d, want 1", doc.Number.Seq)
	}
	if store.WorkCount() != 1 {
		t.Fatalf("work persisted = %d, want 1", store.WorkCount())
	}
	if doc.Hash == "" {
		t.Fatalf("work document missing hash — chain not advanced")
	}
	s, _ := store.GetSeries(testTenantID, "NE2026", domain.NE)
	if s.LastNum != 1 || s.LastHash != string(doc.Hash) {
		t.Fatalf("series not advanced: LastNum=%d LastHash==hash:%v", s.LastNum, s.LastHash == string(doc.Hash))
	}
	if store.OutboxLen() != 0 {
		t.Fatalf("outbox = %d, want 0 (work documents are not communicated)", store.OutboxLen())
	}
}

func TestIssueStockMovement_HappyPath(t *testing.T) {
	now := testNow()
	svc, store := newFixtureSeries(now, activeSeries("GR2026", domain.GR, now))
	draft := stockDraft(activeSeries("GR2026", domain.GR, now), now)

	doc, err := svc.Invoicing.IssueStockMovement(
		context.Background(), testTenantID, draft, "GR2026", "src-1",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"},
	)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if store.StockCount() != 1 {
		t.Fatalf("stock persisted = %d, want 1", store.StockCount())
	}
	if doc.Hash == "" {
		t.Fatalf("stock movement missing hash — chain not advanced")
	}
	s, _ := store.GetSeries(testTenantID, "GR2026", domain.GR)
	if s.LastNum != 1 || s.LastHash != string(doc.Hash) {
		t.Fatalf("series not advanced: LastNum=%d", s.LastNum)
	}
}

func TestIssuePayment_HappyPath(t *testing.T) {
	now := testNow()
	svc, store := newFixtureSeries(now, activeSeries("RG2026", domain.RG, now))
	draft, totals := paymentDraftRG(now)

	doc, err := svc.Invoicing.IssuePayment(
		context.Background(), testTenantID, draft, "RG2026",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}, totals,
	)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if doc.Number.Seq != 1 {
		t.Fatalf("seq = %d, want 1", doc.Number.Seq)
	}
	if store.PaymentCount() != 1 {
		t.Fatalf("payments persisted = %d, want 1", store.PaymentCount())
	}
	// Receipts carry no hash chain (domain.Payment has no Hash field); the series
	// sequence still advances under the optimistic-version guard, but its head
	// hash must stay empty (IssuePayment appends with an empty hash).
	s, _ := store.GetSeries(testTenantID, "RG2026", domain.RG)
	if s.LastNum != 1 {
		t.Fatalf("series LastNum = %d, want 1", s.LastNum)
	}
	if s.LastHash != "" {
		t.Fatalf("receipt advanced series with non-empty hash: %q", s.LastHash)
	}
}

func TestIssuePayment_IdempotentReplay(t *testing.T) {
	now := testNow()
	svc, store := newFixtureSeries(now, activeSeries("RG2026", domain.RG, now))
	draft, totals := paymentDraftRG(now)
	idem := app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}

	first, err := svc.Invoicing.IssuePayment(context.Background(), testTenantID, draft, "RG2026", idem, totals)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.Invoicing.IssuePayment(context.Background(), testTenantID, draft, "RG2026", idem, totals)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.Number != second.Number {
		t.Fatalf("replay returned %s, want %s", second.Number.Format(), first.Number.Format())
	}
	if store.PaymentCount() != 1 {
		t.Fatalf("payments = %d, want 1 (replay must not re-issue)", store.PaymentCount())
	}
}

// TestIssueDebitNote_SuppliesReader proves the service wires a repo-backed reader
// for ND issuance (ND requires IssueOptions.Reader to validate its product set
// against the originating invoice). The FT it references is issued first through
// the same service, so it lives in the document repo when the ND is issued.
func TestIssueDebitNote_SuppliesReader(t *testing.T) {
	now := testNow()
	svc, store := newFixtureSeries(now,
		activeSeries("FT2026", domain.FT, now),
		activeSeries("ND2026", domain.ND, now),
	)

	ft, err := svc.Invoicing.IssueSalesInvoice(
		context.Background(), testTenantID, ftDraft(activeSeries("FT2026", domain.FT, now), now),
		"FT2026", "src-ft", app.IdempotencyKey{Key: "kft", Fingerprint: "fp"},
	)
	if err != nil {
		t.Fatalf("issue FT: %v", err)
	}

	nd, err := svc.Invoicing.IssueSalesInvoice(
		context.Background(), testTenantID, ndDraft(activeSeries("ND2026", domain.ND, now), now, ft),
		"ND2026", "src-nd", app.IdempotencyKey{Key: "knd", Fingerprint: "fp"},
	)
	if err != nil {
		t.Fatalf("issue ND (reader must be supplied by the service): %v", err)
	}
	if nd.Number.Type != domain.ND {
		t.Fatalf("issued type = %s, want ND", nd.Number.Type)
	}
	if store.SalesCount() != 2 {
		t.Fatalf("sales persisted = %d, want 2 (FT + ND)", store.SalesCount())
	}
}

// TestExportSAFT_AllFamilies issues one document of each family and checks the
// SAF-T export projects them all (not just sales).
func TestExportSAFT_AllFamilies(t *testing.T) {
	now := testNow()
	svc, _ := newFixtureSeries(now,
		activeSeries("FT2026", domain.FT, now),
		activeSeries("NE2026", domain.NE, now),
		activeSeries("GR2026", domain.GR, now),
		activeSeries("RG2026", domain.RG, now),
	)
	ctx := context.Background()

	if _, err := svc.Invoicing.IssueSalesInvoice(ctx, testTenantID, ftDraft(activeSeries("FT2026", domain.FT, now), now), "FT2026", "s", app.IdempotencyKey{Key: "1", Fingerprint: "1"}); err != nil {
		t.Fatalf("FT: %v", err)
	}
	if _, err := svc.Invoicing.IssueWorkDocument(ctx, testTenantID, workDraft(activeSeries("NE2026", domain.NE, now), now), "NE2026", "s", app.IdempotencyKey{Key: "2", Fingerprint: "2"}); err != nil {
		t.Fatalf("NE: %v", err)
	}
	if _, err := svc.Invoicing.IssueStockMovement(ctx, testTenantID, stockDraft(activeSeries("GR2026", domain.GR, now), now), "GR2026", "s", app.IdempotencyKey{Key: "3", Fingerprint: "3"}); err != nil {
		t.Fatalf("GR: %v", err)
	}
	pd, pt := paymentDraftRG(now)
	if _, err := svc.Invoicing.IssuePayment(ctx, testTenantID, pd, "RG2026", app.IdempotencyKey{Key: "4", Fingerprint: "4"}, pt); err != nil {
		t.Fatalf("RG: %v", err)
	}

	from := now.AddDate(0, 0, -1)
	to := now.AddDate(0, 0, 1)
	out, err := svc.Export.ExportSAFT(ctx, testTenantID, from, to)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, series := range []string{"FT2026", "NE2026", "GR2026", "RG2026"} {
		if !bytes.Contains(out, []byte(series)) {
			t.Fatalf("SAF-T missing %s — family not projected", series)
		}
	}
}
