package pdf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/flyzard/invoicing.v2/internal/domain"
)

// Currency.Amount is the EUR-equivalent; the printed foreign amount must be
// Amount × ExchangeRate (mirrors the SAF-T CurrencyAmount projection).
func TestCurrencyLabel_NativeAmount(t *testing.T) {
	c := domain.Currency{
		Code:         "USD",
		Amount:       mustMoney(t, 150), // 150.00 EUR
		ExchangeRate: mustRate(t, 1.08),
	}
	got := currencyLabel(c)
	if !strings.Contains(got, "USD 162,00") {
		t.Errorf("currencyLabel = %q, want native amount USD 162,00", got)
	}
	if strings.Contains(got, "150,00") {
		t.Errorf("currencyLabel = %q prints the EUR amount as foreign", got)
	}

	// Half-cent rounding must match the SAF-T projection (NativeAmount,
	// half-away-from-zero): 0.25 € × 0.3 = 0.075 → 0,08, not 0,07.
	half := domain.Currency{Code: "USD", Amount: mustMoney(t, 0.25), ExchangeRate: mustRate(t, 0.3)}
	if got := currencyLabel(half); !strings.Contains(got, "USD 0,08") {
		t.Errorf("currencyLabel = %q, want half-away-from-zero USD 0,08", got)
	}
}

// Labels must come from frozen document data, never the live rate table.
func TestTaxEntryLabel(t *testing.T) {
	vatLine := func(rate domain.TaxRate) domain.DocumentLine {
		return domain.DocumentLine{Tax: domain.VATTax{Rate: rate}}
	}
	cases := []struct {
		name  string
		entry domain.TaxBreakdownEntry
		lines []domain.DocumentLine
		want  string
	}{
		{
			name:  "normal rate from frozen line",
			entry: domain.TaxBreakdownEntry{Region: domain.PT, Category: domain.TaxNormal},
			lines: []domain.DocumentLine{vatLine(domain.TaxRate{
				Region: domain.PT, Category: domain.TaxNormal, Value: mustPercent(t, 23)})},
			want: "IVA 23,00% (PT)",
		},
		{
			name: "exempt prints Isento, not IVA 0,00%",
			entry: domain.TaxBreakdownEntry{
				Region: domain.PT, Category: domain.TaxExempt, ExemptionCode: domain.M07},
			want: "Isento (M07)",
		},
		{
			name: "reverse charge prints autoliquidação",
			entry: domain.TaxBreakdownEntry{
				Region: domain.PT, Category: domain.TaxExempt, ExemptionCode: domain.M30},
			want: "IVA autoliquidação (M30)",
		},
		{
			name:  "OUT prints the line's own frozen value",
			entry: domain.TaxBreakdownEntry{Region: domain.PT, Category: domain.TaxOther},
			lines: []domain.DocumentLine{vatLine(domain.TaxRate{
				Region: domain.PT, Category: domain.TaxOther, Value: mustPercent(t, 4.5)})},
			want: "IVA 4,50% (PT)",
		},
		{
			name:  "no matching line falls back to category",
			entry: domain.TaxBreakdownEntry{Region: domain.PT, Category: domain.TaxNormal},
			want:  "NOR (PT)",
		},
		{
			// OUT rates are caller-supplied and not part of the breakdown
			// key, so one entry can aggregate mixed rates — no single rate
			// may be printed then.
			name:  "mixed OUT rates fall back to category",
			entry: domain.TaxBreakdownEntry{Region: domain.PT, Category: domain.TaxOther},
			lines: []domain.DocumentLine{
				vatLine(domain.TaxRate{Region: domain.PT, Category: domain.TaxOther, Value: mustPercent(t, 4)}),
				vatLine(domain.TaxRate{Region: domain.PT, Category: domain.TaxOther, Value: mustPercent(t, 6)}),
			},
			want: "OUT (PT)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taxEntryLabel(tc.entry, tc.lines); got != tc.want {
				t.Errorf("taxEntryLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

// A zero-amount withholding entry must not print a "-0,00 €" row nor
// suppress/emit a pointless "Total recebido".
func TestPaymentTotals_ZeroWithholdingSkipped(t *testing.T) {
	p := domain.Payment{}
	p.NetTotal = mustMoney(t, 100)
	p.TaxPayable = mustMoney(t, 23)
	p.GrossTotal = mustMoney(t, 123)
	p.WithholdingTax = []domain.WithholdingTax{{Type: domain.WithholdingIRS, Amount: 0}}

	entries := paymentTotals(p)
	for _, e := range entries {
		if strings.Contains(e.label, "Retenção") {
			t.Errorf("zero-amount retention row printed: %+v", e)
		}
		if e.label == "Total recebido" {
			t.Errorf("Total recebido printed with nothing withheld: %+v", e)
		}
	}
}

func TestSalesTotals_ZeroWithholdingSkipped(t *testing.T) {
	tot := domain.Totals{
		NetTotal: mustMoney(t, 100), TaxTotal: mustMoney(t, 23),
		GrossTotal: mustMoney(t, 123), AmountPayable: mustMoney(t, 123),
	}
	entries := salesTotals(tot, []domain.WithholdingTax{{Type: domain.WithholdingIRS, Amount: 0}}, nil)
	for _, e := range entries {
		if strings.Contains(e.label, "Retenção") {
			t.Errorf("zero-amount retention row printed: %+v", e)
		}
	}
}

func TestDisplayBreakdown_OrdersExemptToNormal(t *testing.T) {
	in := domain.TaxBreakdown{
		{Region: domain.PT, Category: domain.TaxNormal},
		{Region: domain.PT, Category: domain.TaxExempt, ExemptionCode: domain.M07},
		{Region: domain.PT, Category: domain.TaxReduced},
		{Region: domain.PT, Category: domain.TaxIntermediate},
	}
	got := displayBreakdown(in)
	want := []domain.TaxCategory{domain.TaxExempt, domain.TaxReduced, domain.TaxIntermediate, domain.TaxNormal}
	for i, e := range got {
		if e.Category != want[i] {
			t.Fatalf("position %d: got %s, want %s (order: %v)", i, e.Category, want[i], got)
		}
	}
	// input slice must NOT be mutated (we sort a clone, the domain order feeds the frozen QR)
	if in[0].Category != domain.TaxNormal {
		t.Errorf("displayBreakdown mutated its input; must clone")
	}
}

func TestLineTaxLabel_ExemptShowsCode(t *testing.T) {
	tx, err := domain.NewVATLineTax(domain.PT, domain.TaxExempt, domain.M07, "Isento artigo 9.º do CIVA")
	if err != nil {
		t.Fatal(err)
	}
	if got := lineTaxLabel(tx); got != "Isento (M07)" {
		t.Errorf("lineTaxLabel exempt = %q, want \"Isento (M07)\"", got)
	}
}

func TestMetaValidate_LogoPNG(t *testing.T) {
	m := validMeta()
	m.LogoPNG = []byte("not a png")
	if err := m.validate(); !errors.Is(err, ErrInvalidLogoPNG) {
		t.Fatalf("want ErrInvalidLogoPNG for garbage bytes, got %v", err)
	}

	m.LogoPNG = []byte{}
	if err := m.validate(); !errors.Is(err, ErrInvalidLogoPNG) {
		t.Fatalf("want ErrInvalidLogoPNG for empty slice, got %v", err)
	}

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.Black)
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	m.LogoPNG = buf.Bytes()
	if err := m.validate(); err != nil {
		t.Fatalf("valid PNG rejected: %v", err)
	}
}
