package saft

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

func TestBuildLine_TaxBaseEmittedWhenSet(t *testing.T) {
	base := must(domain.NewMoney(100.00))
	l := minimalSalesInvoice().Lines[0]
	l.UnitPrice = must(domain.NewMoney(0)) // tax-only line: TaxBase XOR nonzero UnitPrice (XSD assert)
	l.TaxBase = &base

	out := buildLine(l, sideCredit)
	if out.TaxBase == nil {
		t.Fatal("TaxBase element not emitted for a line with TaxBase set")
	}
	if got := domain.Money(*out.TaxBase).Format2DP(); got != "100.00" {
		t.Errorf("TaxBase = %q, want 100.00", got)
	}
	// A tax-only line nets to zero, satisfying the XSD assert (TaxBase ⇒ CreditAmount 0).
	if out.CreditAmount == nil || domain.Money(*out.CreditAmount).Format2DP() != "0.00" {
		t.Errorf("tax-only line CreditAmount = %v, want 0.00", out.CreditAmount)
	}
}

func TestBuildLine_TaxBaseOmittedWhenUnset(t *testing.T) {
	if out := buildLine(minimalSalesInvoice().Lines[0], sideCredit); out.TaxBase != nil {
		t.Errorf("TaxBase should be nil when DocumentLine.TaxBase is unset")
	}
}

// TestLine_CreditAmountIsAuthoritativeNet pins the SAF-T line money convention:
// CreditAmount is the authoritative unrounded net (LineNetAmount); UnitPrice is
// the 5dp effective unit price (informational). For large fractional quantities
// UnitPrice × Quantity can drift from CreditAmount by a few cents — whether AT
// requires them to reconcile within tolerance is tracked as confirm-item C5.
func TestLine_CreditAmountIsAuthoritativeNet(t *testing.T) {
	l := minimalSalesInvoice().Lines[0] // qty 2 × 50.00 -> net 100.00
	out := buildLine(l, sideCredit)
	if out.CreditAmount == nil || domain.Money(*out.CreditAmount) != l.LineNetAmount() {
		t.Fatalf("CreditAmount must equal LineNetAmount (authoritative net)")
	}
	if domain.Money(out.UnitPrice) != l.EffectiveUnitPrice() {
		t.Errorf("UnitPrice must equal EffectiveUnitPrice (5dp informational)")
	}
}
