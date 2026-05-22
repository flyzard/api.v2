package domain

import (
	"testing"
	"time"
)

func TestAddLine_AutoSequences(t *testing.T) {
	d := validDraft(t)
	d.Lines = nil // start clean

	tax, _ := NewVATLineTax(PT, TaxNormal, "", "")
	mk := func(caller int) DocumentLine {
		return DocumentLine{
			LineNumber:   caller, // intentionally wrong; AddLine must overwrite
			Quantity:     mustQuantity(t, 1),
			UnitPrice:    mustMoney(t, 10),
			TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Tax:          tax,
		}
	}
	d.AddLine(mk(99))
	d.AddLine(mk(99))
	d.AddLine(mk(99))

	for i, line := range d.Lines {
		if line.LineNumber != i+1 {
			t.Errorf("line %d: LineNumber=%d, want %d", i, line.LineNumber, i+1)
		}
	}
}

func TestTaxBreakdown_AggregatesByRegionAndCategory(t *testing.T) {
	d := validDraft(t)
	d.Lines = nil

	taxNormalPT, _ := NewVATLineTax(PT, TaxNormal, "", "")
	taxRedAC, _ := NewVATLineTax(PTAC, TaxReduced, "", "")
	taxIseM04, _ := NewVATLineTax(PT, TaxExempt, M04, "Isento")

	mk := func(price float64, tax LineTax) DocumentLine {
		return DocumentLine{
			LineNumber:   1,
			Quantity:     mustQuantity(t, 1),
			UnitPrice:    mustMoney(t, price),
			TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Tax:          tax,
		}
	}
	d.AddLine(mk(100, taxNormalPT)) // PT/NOR
	d.AddLine(mk(50, taxRedAC))     // PT-AC/RED
	d.AddLine(mk(30, taxIseM04))    // PT/ISE/M04
	d.AddLine(mk(20, taxNormalPT))  // PT/NOR (merge into existing entry)

	d.CalculateTotals()

	if got := len(d.Totals.Breakdown); got != 3 {
		t.Fatalf("expected 3 breakdown entries, got %d: %+v", got, d.Totals.Breakdown)
	}

	// Sort order: PT < PT-AC (string compare on region values).
	// Entry 0: PT/ISE M04
	e0 := d.Totals.Breakdown[0]
	if e0.Region != PT || e0.Category != TaxExempt || e0.ExemptionCode != M04 {
		t.Errorf("entry 0: %+v", e0)
	}
	if e0.ExemptionDescription == "" {
		t.Errorf("entry 0: missing exemption description")
	}
	// Entry 1: PT/NOR (€120 merged)
	e1 := d.Totals.Breakdown[1]
	if e1.Region != PT || e1.Category != TaxNormal {
		t.Errorf("entry 1: %+v", e1)
	}
	if want := mustMoney(t, 120); e1.Base != want {
		t.Errorf("PT/NOR base: got %d, want %d", e1.Base, want)
	}
	// Entry 2: PT-AC/RED
	e2 := d.Totals.Breakdown[2]
	if e2.Region != PTAC || e2.Category != TaxReduced {
		t.Errorf("entry 2: %+v", e2)
	}
}

func TestPaymentTerms_RoundtripPropagates(t *testing.T) {
	d := validDraft(t)
	d.Series = *registeredSeries(t, "A", FT)
	due := d.Date.AddDate(0, 0, 30)
	d.PaymentTerms = &due
	if err := d.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestPaymentTerms_RejectsBeforeDocDate(t *testing.T) {
	d := validDraft(t)
	d.Series = *registeredSeries(t, "A", FT)
	past := d.Date.AddDate(0, 0, -1)
	d.PaymentTerms = &past
	if err := d.Validate(); err == nil {
		t.Fatal("expected error when payment terms precede document date")
	}
}

func TestTotals_AmountPayableDefaultsToGross(t *testing.T) {
	d := validDraft(t)
	d.CalculateTotals()
	if d.Totals.AmountPayable != d.Totals.GrossTotal {
		t.Errorf("AmountPayable: got %v, want %v", d.Totals.AmountPayable, d.Totals.GrossTotal)
	}
}

func TestTotals_AmountPayableSubtractsWithholding(t *testing.T) {
	d := validDraft(t)
	d.CalculateTotals()
	wht := []WithholdingTax{
		{Type: WithholdingIRS, Amount: mustMoney(t, 23)},
	}
	d.Totals.applyWithholding(wht)
	want := d.Totals.GrossTotal - mustMoney(t, 23)
	if d.Totals.AmountPayable != want {
		t.Errorf("AmountPayable: got %v, want %v", d.Totals.AmountPayable, want)
	}
}

func TestCalculateTotals_TaxBaseOverridesUnitPrice(t *testing.T) {
	d := validDraft(t)
	d.Lines = nil

	tax, _ := NewVATLineTax(PT, TaxNormal, "", "")
	base := mustMoney(t, 100)
	d.AddLine(DocumentLine{
		LineNumber:   1,
		Quantity:     mustQuantity(t, 1),
		UnitPrice:    0,
		TaxBase:      &base,
		TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Tax:          tax,
	})

	d.CalculateTotals()

	if d.Totals.NetTotal != 0 {
		t.Errorf("NetTotal: got %v, want 0", d.Totals.NetTotal)
	}
	if want := mustMoney(t, 23); d.Totals.TaxTotal != want {
		t.Errorf("TaxTotal: got %v, want %v", d.Totals.TaxTotal, want)
	}
	if want := mustMoney(t, 23); d.Totals.GrossTotal != want {
		t.Errorf("GrossTotal: got %v, want %v", d.Totals.GrossTotal, want)
	}
	if len(d.Totals.Breakdown) != 1 || d.Totals.Breakdown[0].Base != base {
		t.Errorf("breakdown should record TaxBase=100, got %+v", d.Totals.Breakdown)
	}
}

func TestTaxBreakdown_SumsMatchTotals(t *testing.T) {
	d := validDraft(t)
	d.CalculateTotals()
	var baseSum, taxSum Money
	for _, e := range d.Totals.Breakdown {
		baseSum += e.Base
		taxSum += e.Tax
	}
	if baseSum != d.Totals.NetTotal {
		t.Errorf("Σ base %d != NetTotal %d", baseSum, d.Totals.NetTotal)
	}
	if taxSum != d.Totals.TaxTotal {
		t.Errorf("Σ tax %d != TaxTotal %d", taxSum, d.Totals.TaxTotal)
	}
}

func TestValidate_RejectsDuplicateLineNumber(t *testing.T) {
	d := validDraft(t)
	tax, _ := NewVATLineTax(PT, TaxNormal, "", "")
	d.Lines = append(d.Lines, DocumentLine{
		LineNumber:   1, // duplicate of existing first line
		Quantity:     mustQuantity(t, 1),
		UnitPrice:    mustMoney(t, 10),
		TaxPointDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Tax:          tax,
	})
	if err := d.Validate(); err == nil {
		t.Fatal("expected duplicate LineNumber to be rejected")
	}
}
