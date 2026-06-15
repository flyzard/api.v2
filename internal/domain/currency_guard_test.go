package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// normalVATLine builds a one-line NOR-rate fixture for issuance tests.
func normalVATLine(date time.Time) DocumentLine {
	return DocumentLine{
		LineNumber: 1,
		Product: mustVal(NewProduct(Product{
			ProductCode:        "P-1",
			ProductType:        ProductTypeGoods,
			ProductDescription: "Mercadoria de teste",
			ProductNumberCode:  "P-1",
			Unit:               UnitPiece,
		})),
		Quantity:     mustVal(NewQuantity(1)),
		UnitPrice:    mustVal(NewMoney(10)),
		TaxPointDate: date,
		Tax:          mustVal(NewVATLineTax(PT, TaxNormal, "", "")),
	}
}

// TestCurrencyNativeAmount pins the integer reconstruction of the native
// FX amount (Amount × ExchangeRate) with half-away-from-zero rounding — the
// same convention as Format2DP. A float round-trip lands on half-cents
// unpredictably (binary representation) and %.2f rounds half-to-even.
func TestCurrencyNativeAmount(t *testing.T) {
	cases := []struct {
		amount float64
		rate   float64
		want   string // Format2DP of the native amount
	}{
		{0.25, 0.3, "0.08"},         // 7.5 cents: half rounds away, float gave 0.07
		{10.00, 1.085, "10.85"},     // exact product
		{347.20, 1.0, "347.20"},     // identity rate
		{100.00, 0.123456, "12.35"}, // 12.3456 rounds up
	}
	for _, c := range cases {
		cur := Currency{
			Code:         "USD",
			Amount:       mustVal(NewMoney(c.amount)),
			ExchangeRate: mustVal(NewExchangeRate(c.rate)),
			Date:         time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC),
		}
		if got := cur.NativeAmount().Format2DP(); got != c.want {
			t.Errorf("NativeAmount(%v × %v) = %s, want %s", c.amount, c.rate, got, c.want)
		}
	}
}

// TestCurrencyDateGuardNormalizesToLisbon pins the FX rate-date guard to
// calendar-day comparison in Europe/Lisbon. dateOnly keeps each operand's own
// location and time.Equal compares instants, so before the fix a UTC-located
// Currency.Date on the same calendar day was rejected whenever Lisbon was on
// summer time (UTC+1) and accepted in winter (UTC+0) — a DST-dependent guard.
func TestCurrencyDateGuardNormalizesToLisbon(t *testing.T) {
	now := time.Date(2026, 6, 5, 15, 0, 0, 0, time.UTC) // June: Lisbon = UTC+1
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}
	currency := func(day int) *Currency {
		return &Currency{
			Code:         "USD",
			Amount:       Money(10 * scale),
			ExchangeRate: ExchangeRate(1_085_000), // 1.085
			Date:         time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC),
		}
	}

	salesDraft := func(series Series, cur *Currency) *DraftSalesInvoice {
		draft := &DraftSalesInvoice{}
		draft.DocumentType = FT
		draft.Customer = Customer{
			CustomerID:    uuid.New(),
			AccountID:     "ACC-CG",
			CustomerTaxID: "500000000",
			CompanyName:   "Cliente CG Lda.",
			BillingAddress: Address{
				AddressDetail: "Rua de Teste 1",
				City:          "Lisboa",
				PostalCode:    "1000-001",
				Country:       "PT",
			},
		}
		draft.Series = series
		draft.Date = now
		draft.Lines = []DocumentLine{normalVATLine(now)}
		draft.Currency = cur
		return draft
	}

	t.Run("sales-same-day-utc-accepted", func(t *testing.T) {
		series := registeredSeries(t)
		draft := salesDraft(series, currency(5))
		if _, err := IssueSalesInvoice(draft, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr); err != nil {
			t.Fatalf("same-calendar-day UTC currency date rejected: %v", err)
		}
	})

	t.Run("sales-different-day-rejected", func(t *testing.T) {
		series := registeredSeries(t)
		draft := salesDraft(series, currency(4))
		_, err := IssueSalesInvoice(draft, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
		if err == nil || !strings.Contains(err.Error(), "currency rate date") {
			t.Fatalf("error = %v, want currency rate date mismatch", err)
		}
	})

	t.Run("payment-same-day-utc-accepted", func(t *testing.T) {
		series := mustVal(NewSeries("RG26", RG))
		if err := series.RegisterWithAT("BCDFGH37", seriesT0); err != nil {
			t.Fatalf("RegisterWithAT: %v", err)
		}
		draft := validPaymentDraft()
		draft.TransactionDate = now
		draft.Currency = currency(5)
		totals := PaymentTotals{NetTotal: Money(10 * scale), GrossTotal: Money(10 * scale)}
		if _, err := IssuePayment(draft, &series, now, totals, IssueOptions{}); err != nil {
			t.Fatalf("same-calendar-day UTC currency date rejected: %v", err)
		}
	})
}
