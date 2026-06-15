package app_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/flyzard/invoicing.v2/internal/app"
)

func TestExportSAFT_ContainsInPeriodInvoice(t *testing.T) {
	svc, _ := newCommFixture()
	draft := ftDraft(activeFTSeries(testNow()), testNow())
	if _, err := svc.Invoicing.IssueSalesInvoice(context.Background(), testTenantID, draft, "FT2026", "src-1",
		app.IdempotencyKey{Key: "k1", Fingerprint: "fp1"}); err != nil {
		t.Fatalf("issue: %v", err)
	}

	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, lisbon)
	to := time.Date(2026, 5, 31, 23, 59, 59, 0, lisbon)
	out, err := svc.Export.ExportSAFT(context.Background(), testTenantID, from, to)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("export produced no bytes")
	}
	if !bytes.Contains(out, []byte("FT2026")) {
		t.Fatal("export XML does not contain the issued series")
	}
}
