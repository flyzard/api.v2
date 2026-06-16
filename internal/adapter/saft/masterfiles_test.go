package saft

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
	"github.com/google/uuid"
)

// TestExport_CurrencyAmountHalfAwayRounding pins CurrencyAmount to the integer
// half-away-from-zero convention every other monetary field uses. The old
// float path (Amount.Float64() × rate, %.2f) rounded half-to-even and drifted
// on half-cents: GrossTotal €0.25 × rate 0.3 = 7.5 cents must render 0.08;
// CurrencyAmount is now derived from GrossTotal × rate (not Currency.Amount).
func TestExport_CurrencyAmountHalfAwayRounding(t *testing.T) {
	inv := minimalSalesInvoice()
	// Override totals so GrossTotal = €0.25 (the half-cent rounding fixture).
	inv.Totals.NetTotal = must(domain.NewMoney(0.25))
	inv.Totals.TaxTotal = must(domain.NewMoney(0.00))
	inv.Totals.GrossTotal = must(domain.NewMoney(0.25))
	inv.Currency = &domain.Currency{
		Code:         "USD",
		Amount:       must(domain.NewMoney(0.25)), // must equal GrossTotal
		ExchangeRate: must(domain.NewExchangeRate(0.3)),
		Date:         inv.Date,
	}
	out, err := Export(gdTestHeader(), []domain.SalesInvoice{inv}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	body := decodeWin1252(t, out)
	if want := "<CurrencyAmount>0.08</CurrencyAmount>"; !strings.Contains(body, want) {
		t.Errorf("missing %s in:\n%s", want, snippetAround(body, "<Currency>"))
	}
	if want := "<ExchangeRate>0.300000</ExchangeRate>"; !strings.Contains(body, want) {
		t.Errorf("missing %s", want)
	}
}

// snippetAround returns ±200 bytes around the first occurrence of marker.
func snippetAround(body, marker string) string {
	i := strings.Index(body, marker)
	if i < 0 {
		return body
	}
	lo := max(0, i-200)
	hi := min(len(body), i+200)
	return body[lo:hi]
}

// TestExport_CustomerOrderDeterministic pins byte-stable output when two
// customers share an AccountID (a GL account like "Desconhecido" is not
// unique). Customers are deduplicated by CustomerID — the only key guaranteed
// unique — so the export order must come from it too; sorting by AccountID
// left equal keys in map-iteration order, flipping bytes between runs.
func TestExport_CustomerOrderDeterministic(t *testing.T) {
	inv1 := minimalSalesInvoice()

	inv2 := minimalSalesInvoice()
	inv2.Number = must(domain.NewDocNumber(domain.FT, "FT2026", 2))
	inv2.Customer.CustomerID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	inv2.Customer.CompanyName = "Beta Faturação Lda."
	// Same AccountID as inv1's customer — the non-unique sort key under test.

	hdr := gdTestHeader()
	docs := []domain.SalesInvoice{inv1, inv2}

	first, err := Export(hdr, docs, nil, nil, nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for i := range 30 {
		out, err := Export(hdr, docs, nil, nil, nil)
		if err != nil {
			t.Fatalf("Export #%d: %v", i, err)
		}
		if !bytes.Equal(out, first) {
			t.Fatalf("Export #%d differs from first run — customer order nondeterministic with shared AccountID", i)
		}
	}
}
