package saft

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// TestBuildPayments_MovementWalk pins the debit/credit aggregate walk:
// TotalDebit/TotalCredit sum per movement side, exclude cancelled payments,
// but NumberOfEntries counts everything (Portaria 302/2016 field rules).
// The golden test guards the bytes; this one localizes aggregate regressions.
func TestBuildPayments_MovementWalk(t *testing.T) {
	_, _, _, payments := goldenDocs() // [0] normal RG (credit 110 + debit 10), [1] cancelled clone

	out := buildPayments(payments)

	if out.NumberOfEntries != 2 {
		t.Errorf("NumberOfEntries = %d, want 2 (cancelled stays counted)", out.NumberOfEntries)
	}
	if got := domain.Money(out.TotalCredit).Format2DP(); got != "110.00" {
		t.Errorf("TotalCredit = %s, want 110.00 (cancelled excluded)", got)
	}
	if got := domain.Money(out.TotalDebit).Format2DP(); got != "10.00" {
		t.Errorf("TotalDebit = %s, want 10.00 (cancelled excluded)", got)
	}

	// Line-level movement dispatch: credit line → CreditAmount, debit → DebitAmount.
	lines := out.Payments[0].Lines
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0].CreditAmount == nil || lines[0].DebitAmount != nil {
		t.Errorf("line 1: want CreditAmount only, got debit=%v credit=%v", lines[0].DebitAmount, lines[0].CreditAmount)
	}
	if lines[1].DebitAmount == nil || lines[1].CreditAmount != nil {
		t.Errorf("line 2: want DebitAmount only, got debit=%v credit=%v", lines[1].DebitAmount, lines[1].CreditAmount)
	}

	// Cancelled payment carries status A + reason in DocumentStatus.
	st := out.Payments[1].DocumentStatus
	if st.PaymentStatus != "A" || st.Reason == "" {
		t.Errorf("cancelled status = %+v, want PaymentStatus A with Reason", st)
	}
}
