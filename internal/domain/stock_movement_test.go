package domain

import (
	"strings"
	"testing"
	"time"
)

// gtDraft builds a valid GT draft whose MovementStartTime is now+startOffset.
func gtDraft(t *testing.T, series Series, now time.Time, startOffset time.Duration) *DraftStockMovement {
	t.Helper()
	addr := mustVal(NewAddress("Armazem 1", "Lisboa", "1000-001", "PT"))
	draft := &DraftStockMovement{}
	draft.DocumentType = GT
	draft.Customer = *mustVal(NewCustomer("ACC-1", "555555550", "Cliente Lda", addr, false))
	draft.Date = now
	draft.Series = series
	draft.MovementStartTime = now.Add(startOffset)
	draft.ShipFrom = &ShippingPoint{Address: &addr}
	draft.ShipTo = &ShippingPoint{Address: &addr}
	draft.AddLine(normalVATLine(now))
	return draft
}

// TestIssueStockMovementRejectionLeavesSeriesUntouched pins issueCommon's
// contract ("on error the series is untouched") for the F-SAFT-16 guard: a
// guia whose movement already started before system entry must be rejected
// BEFORE the series counter advances — otherwise the rejection burns a
// sequence number and the hash chain references a document that was never
// returned to the caller.
func TestIssueStockMovementRejectionLeavesSeriesUntouched(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}

	series := mustVal(NewSeries("GT26", GT))
	if err := series.RegisterWithAT("BCDFGH38", now); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}

	draft := gtDraft(t, series, now, -2*time.Hour) // goods moved before issue
	_, err := IssueStockMovement(draft, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
	if err == nil || !strings.Contains(err.Error(), "movement_start_time") {
		t.Fatalf("error = %v, want movement_start_time precedes system entry", err)
	}
	if series.LastNum != 0 {
		t.Errorf("series.LastNum = %d after rejected issue, want 0 (series must be untouched)", series.LastNum)
	}
	if series.LastHash != "" {
		t.Errorf("series.LastHash = %q after rejected issue, want empty (fresh series)", series.LastHash)
	}

	// Sanity: the same draft with a future start issues fine and advances the series.
	ok := gtDraft(t, series, now, 2*time.Hour)
	if _, err := IssueStockMovement(ok, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr); err != nil {
		t.Fatalf("valid guia rejected: %v", err)
	}
	if series.LastNum != 1 {
		t.Errorf("series.LastNum = %d after valid issue, want 1", series.LastNum)
	}
}
