package saft

import (
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// TestRecon_ExportedGrossEqualsDomain pins the property AT relies on to
// re-verify signatures: the projected <GrossTotal> equals the domain
// Totals.GrossTotal for every matrix sales case. GREEN — do not let it drift.
func TestRecon_ExportedGrossEqualsDomain(t *testing.T) {
	for _, c := range matrixDocs() {
		if len(c.sales) != 1 {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			out := mustExport(t, c.sales, nil, nil, nil)
			_, _, grossCents := parseDocTotals(t, out)
			wantCents := cents2dp(t, c.sales[0].Totals.GrossTotal.Format2DP())
			if grossCents != wantCents {
				t.Errorf("exported GrossTotal %d cents != domain %d cents", grossCents, wantCents)
			}
		})
	}
}

// TestRecon_NetPlusTaxEqualsGross asserts the DocumentTotals identity
// round(Net)+round(Tax) == round(Gross). PARKED: today independent 2dp rounding
// of sub-cent accumulators breaks it. Phase 3 (M-Recon) un-skips this.
func TestRecon_NetPlusTaxEqualsGross(t *testing.T) {
	// Sub-cent accumulators (scale 100000): Net 0.01250, Tax 0.00288, Gross 0.01538
	// → 2dp: 0.01 + 0.00 != 0.02. Reachable via fractional quantities × 0.01 prices.
	inv := minimalSalesInvoice()
	inv.Totals = domain.Totals{
		NetTotal:   domain.Money(1250),
		TaxTotal:   domain.Money(288),
		GrossTotal: domain.Money(1538),
	}
	out := mustExport(t, []domain.SalesInvoice{inv}, nil, nil, nil)
	net, tax, gross := parseDocTotals(t, out)
	if net+tax != gross {
		t.Errorf("Net %d + Tax %d != Gross %d cents (2dp reconciliation drift)", net, tax, gross)
	}
}

// TestRecon_CurrencyAmountEqualsGross asserts the exported <CurrencyAmount>
// equals the document gross expressed in the original currency (GrossTotal ×
// ExchangeRate). PARKED: today the projector derives CurrencyAmount from the
// unbound Currency.Amount, so a wrong Amount exports a wrong figure (the
// TestExport_CurrencyDirection net-as-gross hazard). Phase 3 (M-Currency)
// un-skips this once CurrencyAmount is derived from GrossTotal.
func TestRecon_CurrencyAmountEqualsGross(t *testing.T) {
	rate := must(domain.NewExchangeRate(1.085))
	inv := minimalSalesInvoice() // GrossTotal 123.00
	inv.Currency = &domain.Currency{
		Code:         "USD",
		Amount:       must(domain.NewMoney(100.00)), // WRONG on purpose: net-as-gross (the M-Currency bug)
		ExchangeRate: rate,
		Date:         inv.Date,
	}
	out := mustExport(t, []domain.SalesInvoice{inv}, nil, nil, nil)

	got := firstTagCents(t, out, "CurrencyAmount")
	// Correct value: the document gross expressed in the original currency.
	want := cents2dp(t, domain.Currency{Amount: inv.Totals.GrossTotal, ExchangeRate: rate, Date: inv.Date}.NativeAmount().Format2DP())
	if got != want {
		t.Errorf("exported CurrencyAmount %d cents != GrossTotal×rate %d cents", got, want)
	}
}
