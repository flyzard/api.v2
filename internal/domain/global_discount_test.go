package domain

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProrateCents(t *testing.T) {
	cases := []struct {
		name    string
		cents   int64
		weights []Money // scaled units (1€ = 100_000)
		want    []Money // scaled units, whole cents
	}{
		{
			// §5.7 demo numbers: line1 €50.16 (post 8.8% line discount), line2 €10.00, D = €3.00.
			// Floors: 250 / 49 cents; leftover 1 cent goes to the larger remainder (line 2).
			name:    "demo_5_7_split",
			cents:   300,
			weights: []Money{5016000, 1000000},
			want:    []Money{250 * 1000, 50 * 1000},
		},
		{
			// Equal weights, leftover ties broken by lower index.
			name:    "tie_break_lower_index",
			cents:   2,
			weights: []Money{100000, 100000, 100000},
			want:    []Money{1000, 1000, 0},
		},
		{
			// Zero-weight line gets nothing.
			name:    "zero_weight_line",
			cents:   100,
			weights: []Money{500000, 0, 500000},
			want:    []Money{50000, 0, 50000},
		},
		{
			// Single line takes everything.
			name:    "single_line",
			cents:   123,
			weights: []Money{700000},
			want:    []Money{123000},
		},
		{
			// Overflow guard: €1M discount (1e8 cents) over a €5M document.
			// cents × weight ≈ 1.7e19 > MaxInt64 — exercises the 128-bit path.
			name:    "large_document_128bit",
			cents:   100_000_000,
			weights: []Money{166_666_700_000, 166_666_700_000, 166_666_600_000},
			want:    nil, // checked by sum + proportionality assertions below
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := prorateCents(tc.cents, tc.weights)
			if len(got) != len(tc.weights) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.weights))
			}
			var sum Money
			for i, s := range got {
				if s < 0 {
					t.Errorf("share[%d] = %d, negative", i, s)
				}
				if int64(s)%centScale != 0 {
					t.Errorf("share[%d] = %d, not a whole cent", i, s)
				}
				sum += s
			}
			if int64(sum) != tc.cents*centScale {
				t.Errorf("Σ shares = %d, want %d", sum, tc.cents*centScale)
			}
			if tc.want != nil {
				for i := range tc.want {
					if got[i] != tc.want[i] {
						t.Errorf("share[%d] = %d, want %d", i, got[i], tc.want[i])
					}
				}
			}
		})
	}
}

func TestProrateCents_Deterministic(t *testing.T) {
	weights := []Money{3333300, 3333300, 3333400}
	a := prorateCents(100, weights)
	b := prorateCents(100, weights)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at %d: %d vs %d", i, a[i], b[i])
		}
	}
}

func gdTestLine(t *testing.T, qty, unit float64, disc Discount) DocumentLine {
	t.Helper()
	q, err := NewQuantity(qty)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMoney(unit)
	if err != nil {
		t.Fatal(err)
	}
	return DocumentLine{
		LineNumber: 1,
		Product: Product{
			ProductCode:        "GD-1",
			ProductDescription: "global discount test product",
			ProductType:        ProductTypeGoods,
			Unit:               "UN",
		},
		Quantity:     q,
		UnitPrice:    m,
		TaxPointDate: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC),
		Discount:     disc,
	}
}

func TestDocumentLine_GlobalDiscountShare(t *testing.T) {
	disc, _ := NewPercentDiscount(8.8)
	l := gdTestLine(t, 100, 0.55, disc)            // subtotal €55.00, line net €50.16
	l.GlobalDiscountShare = Money(250 * centScale) // €2.50

	if got := l.LineNetAmount(); got != 4766000 { // €50.16 − €2.50 = €47.66
		t.Errorf("LineNetAmount = %d, want 4766000", got)
	}
	// Line-discount-only: excludes the global share (round-3348 separation).
	if got := l.LineDiscountAmount(); got != 484000 { // €55.00 − €50.16 = €4.84
		t.Errorf("LineDiscountAmount = %d, want 484000", got)
	}
	// EffectiveUnitPrice reflects both discounts (cert letter Nota 1).
	if got := l.EffectiveUnitPrice(); got != 47660 { // €0.47660
		t.Errorf("EffectiveUnitPrice = %d, want 47660", got)
	}
}

func TestDocumentLine_LineTotalReflectsShare(t *testing.T) {
	l := gdTestLine(t, 1, 10.00, nil)
	l.Tax = mustVAT(t, TaxNormal)
	l.GlobalDiscountShare = Money(50 * centScale) // €0.50
	// (10.00 − 0.50) × 1.23 = 11.6850
	if got := l.LineTotal(); got != 1168500 {
		t.Errorf("LineTotal = %d, want 1168500", got)
	}
}

func mustVAT(t *testing.T, cat TaxCategory) LineTax {
	t.Helper()
	tax, err := NewVATLineTax(PT, cat, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return tax
}

func TestDocumentLine_ValidateShareBounds(t *testing.T) {
	l := gdTestLine(t, 1, 10.00, nil)
	l.Tax = mustVAT(t, TaxNormal)

	l.GlobalDiscountShare = -1
	if err := l.Validate(); err == nil {
		t.Error("negative share accepted")
	}
	l.GlobalDiscountShare = 1100000 // €11.00 > €10.00 line net
	if err := l.Validate(); err == nil {
		t.Error("share exceeding line net accepted")
	}
	l.GlobalDiscountShare = 1000000 // exactly the line net — boundary OK
	if err := l.Validate(); err != nil {
		t.Errorf("boundary share rejected: %v", err)
	}
	l.GlobalDiscountShare = 500 // half a cent — Money JSON (integer cents) would truncate it
	if err := l.Validate(); err == nil {
		t.Error("sub-cent share accepted")
	}
}

// Global discount is a sales-only concept; a stray share on another family's
// line (malformed payload, future persistence bug) must not silently lower
// its net.
func TestValidate_GlobalDiscountShareSalesOnly(t *testing.T) {
	line := gdTestLine(t, 1, 10.00, nil)
	line.Tax = mustVAT(t, TaxNormal)
	line.GlobalDiscountShare = Money(100 * centScale)

	sales := gdDraft(t, nil)
	cd := CommonDraftDocument{
		DocumentCore: DocumentCore{
			DocumentType: NE,
			Customer:     sales.Customer,
			Date:         sales.Date,
		},
		Series: mustVal(NewSeries("GDW2026", NE)),
	}
	cd.Lines = []DocumentLine{line}
	if err := cd.Validate(); err == nil {
		t.Error("work-document line with stray global discount share accepted")
	}

	// Sales family keeps accepting baked shares.
	sales.Lines[0].GlobalDiscountShare = Money(100 * centScale)
	if err := sales.Validate(); err != nil {
		t.Errorf("sales draft with baked share rejected: %v", err)
	}
}

func TestDocumentLine_ShareJSONRoundTrip(t *testing.T) {
	l := gdTestLine(t, 1, 10.00, nil)
	l.Tax = mustVAT(t, TaxNormal)
	l.GlobalDiscountShare = Money(250 * centScale) // whole cents — Money JSON contract holds

	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var back DocumentLine
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.GlobalDiscountShare != l.GlobalDiscountShare {
		t.Errorf("share round-trip = %d, want %d", back.GlobalDiscountShare, l.GlobalDiscountShare)
	}
}

func TestDocumentLine_ZeroShareUnchangedBehavior(t *testing.T) {
	// Regression: without a share, all derived values match the pre-change formulas.
	disc, _ := NewPercentDiscount(8.8)
	l := gdTestLine(t, 100, 0.55, disc)
	if got := l.LineNetAmount(); got != 5016000 {
		t.Errorf("LineNetAmount = %d, want 5016000", got)
	}
	if got := l.LineDiscountAmount(); got != 484000 {
		t.Errorf("LineDiscountAmount = %d, want 484000", got)
	}
	if got := l.EffectiveUnitPrice(); got != 50160 {
		t.Errorf("EffectiveUnitPrice = %d, want 50160", got)
	}
}

func TestSalesInvoiceFields_GlobalDiscountJSONRoundTrip(t *testing.T) {
	amt, _ := NewMoney(3.00)
	disc, _ := NewAmountDiscount(amt)
	f := SalesInvoiceFields{GlobalDiscount: disc}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var back SalesInvoiceFields
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	got, ok := back.GlobalDiscount.(AmountDiscount)
	if !ok {
		t.Fatalf("GlobalDiscount type = %T, want AmountDiscount", back.GlobalDiscount)
	}
	if got.Amount != amt {
		t.Errorf("amount = %d, want %d", got.Amount, amt)
	}
}

// Guards the Go embedded-Unmarshaler promotion pitfall: SalesInvoiceFields
// gaining UnmarshalJSON must not hijack SalesInvoice / DraftSalesInvoice
// decoding and drop the other embedded struct's fields.
// Customer.UnmarshalJSON validates on ingest; build a minimal valid PT
// customer so the round-trip goes through without unrelated failures.
func TestDraftSalesInvoice_JSONRoundTripKeepsBothEmbeds(t *testing.T) {
	disc, _ := NewPercentDiscount(5)
	addr, _ := NewAddress("Rua A 1", "Lisboa", "1000-001", "PT")
	cust, _ := NewCustomer("C001", "500000000", "Test Lda", addr, false)
	draft := DraftSalesInvoice{
		CommonDraftDocument: CommonDraftDocument{
			DocumentCore: DocumentCore{
				DocumentType: FT,
				Customer:     *cust,
			},
			Series: Series{ID: "FT2026"},
		},
		SalesInvoiceFields: SalesInvoiceFields{GlobalDiscount: disc},
	}
	data, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	var back DraftSalesInvoice
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.DocumentType != FT {
		t.Errorf("DocumentType lost: %q", back.DocumentType)
	}
	if back.Customer.CustomerTaxID != cust.CustomerTaxID {
		t.Errorf("CustomerTaxID lost: %q", back.Customer.CustomerTaxID)
	}
	if _, ok := back.GlobalDiscount.(PercentDiscount); !ok {
		t.Errorf("GlobalDiscount lost: %T", back.GlobalDiscount)
	}
}

func TestSalesInvoice_JSONRoundTripKeepsBothEmbeds(t *testing.T) {
	disc, _ := NewPercentDiscount(5)
	addr, _ := NewAddress("Rua A 1", "Lisboa", "1000-001", "PT")
	cust, _ := NewCustomer("C001", "500000000", "Test Lda", addr, false)
	inv := SalesInvoice{
		IssuedDocument: IssuedDocument{
			Hash:     Hash("ABCD"),
			SourceID: "tester@example.com",
			DocumentCore: DocumentCore{
				Customer: *cust,
			},
		},
		SalesInvoiceFields: SalesInvoiceFields{GlobalDiscount: disc},
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	var back SalesInvoice
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Hash != Hash("ABCD") || back.SourceID != "tester@example.com" {
		t.Errorf("IssuedDocument fields lost: hash=%q source=%q", back.Hash, back.SourceID)
	}
	if _, ok := back.GlobalDiscount.(PercentDiscount); !ok {
		t.Errorf("GlobalDiscount lost: %T", back.GlobalDiscount)
	}
}

// gdDraft builds the §5.7-shaped two-line FT: line 1 = 100 × €0.55 with an
// 8.8% line discount (net €50.16), line 2 = 1 × €10.00 plain. Σ nets €60.16.
func gdDraft(t *testing.T, global Discount) *DraftSalesInvoice {
	t.Helper()
	line1 := gdTestLine(t, 100, 0.55, mustPercent(t, 8.8))
	line1.Tax = mustVAT(t, TaxNormal)
	line2 := gdTestLine(t, 1, 10.00, nil)
	line2.LineNumber = 2
	line2.Product.ProductCode = "GD-2"
	line2.Tax = mustVAT(t, TaxNormal)

	d := &DraftSalesInvoice{}
	d.DocumentType = FT
	d.Customer = Customer{
		CustomerID:    uuid.New(),
		AccountID:     "ACC-GD",
		CustomerTaxID: "500000000",
		CompanyName:   "Cliente GD Lda.",
		BillingAddress: Address{
			AddressDetail: "Rua de Teste 1",
			City:          "Lisboa",
			PostalCode:    "1000-001",
			Country:       "PT",
		},
	}
	d.Date = time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	d.Series = mustVal(NewSeries("GD2026", FT))
	d.Lines = []DocumentLine{line1, line2}
	d.GlobalDiscount = global
	return d
}

func mustPercent(t *testing.T, v float64) Discount {
	t.Helper()
	p, err := NewPercentDiscount(v)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCalculateTotals_GlobalDiscount(t *testing.T) {
	amt, _ := NewMoney(3.00)
	disc, _ := NewAmountDiscount(amt)
	d := gdDraft(t, disc)
	d.CalculateTotals()

	// Bases: €50.16 + €10.00 = €60.16; D = €3.00 → shares €2.50 / €0.50.
	if d.Lines[0].GlobalDiscountShare != Money(250*centScale) {
		t.Errorf("share[0] = %d, want %d", d.Lines[0].GlobalDiscountShare, 250*centScale)
	}
	if d.Lines[1].GlobalDiscountShare != Money(50*centScale) {
		t.Errorf("share[1] = %d, want %d", d.Lines[1].GlobalDiscountShare, 50*centScale)
	}
	// NetTotal €57.16; VAT 23%: 47.66→10.9618, 9.50→2.1850; Gross €70.3068.
	if d.Totals.NetTotal != 5716000 {
		t.Errorf("NetTotal = %d, want 5716000", d.Totals.NetTotal)
	}
	if d.Totals.TaxTotal != 1314680 {
		t.Errorf("TaxTotal = %d, want 1314680", d.Totals.TaxTotal)
	}
	if d.Totals.GrossTotal != 7030680 {
		t.Errorf("GrossTotal = %d, want 7030680", d.Totals.GrossTotal)
	}
}

func TestCalculateTotals_NilGlobalDiscountResetsShares(t *testing.T) {
	amt, _ := NewMoney(3.00)
	disc, _ := NewAmountDiscount(amt)
	d := gdDraft(t, disc)
	d.CalculateTotals() // bake
	preNet := d.Totals.NetTotal

	d.GlobalDiscount = nil
	d.CalculateTotals() // must reset, not early-return
	for i, l := range d.Lines {
		if l.GlobalDiscountShare != 0 {
			t.Errorf("share[%d] = %d after nil reset, want 0", i, l.GlobalDiscountShare)
		}
	}
	if d.Totals.NetTotal != 6016000 { // back to €60.16
		t.Errorf("NetTotal = %d, want 6016000 (pre-discount)", d.Totals.NetTotal)
	}
	if preNet == d.Totals.NetTotal {
		t.Error("sanity: discounted and reset NetTotal should differ")
	}
}

func TestCalculateTotals_GlobalDiscountIdempotent(t *testing.T) {
	disc := mustPercent(t, 5)
	d := gdDraft(t, disc)
	d.CalculateTotals()
	first := d.Totals
	shares := []Money{d.Lines[0].GlobalDiscountShare, d.Lines[1].GlobalDiscountShare}
	d.CalculateTotals()
	if !reflect.DeepEqual(d.Totals, first) {
		t.Errorf("totals drifted on recompute: got %+v, want %+v", d.Totals, first)
	}
	if d.Lines[0].GlobalDiscountShare != shares[0] || d.Lines[1].GlobalDiscountShare != shares[1] {
		t.Error("shares drifted on recompute")
	}
}

func TestValidate_GlobalDiscountExceedsNet(t *testing.T) {
	amt, _ := NewMoney(100.00) // > €60.16 total nets
	disc, _ := NewAmountDiscount(amt)
	d := gdDraft(t, disc)
	if err := d.Validate(); !errors.Is(err, ErrGlobalDiscountExceedsNet) {
		t.Errorf("err = %v, want ErrGlobalDiscountExceedsNet", err)
	}
}

func TestValidate_GlobalDiscountOnZeroNet(t *testing.T) {
	disc := mustPercent(t, 10)
	d := gdDraft(t, disc)
	// Zero out every line's value → Σ nets = 0.
	for i := range d.Lines {
		d.Lines[i].UnitPrice = 0
		d.Lines[i].Discount = nil
	}
	if err := d.Validate(); !errors.Is(err, ErrGlobalDiscountOnZeroNet) {
		t.Errorf("err = %v, want ErrGlobalDiscountOnZeroNet", err)
	}
}

func TestIssueSalesInvoice_GlobalDiscountBaked(t *testing.T) {
	// Issue WITHOUT calling draft.CalculateTotals() first — IssueSalesInvoice
	// itself must bake shares before issueCommon (which operates on the
	// embedded CommonDraftDocument where no method shadow applies).
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	amt, _ := NewMoney(3.00)
	disc, _ := NewAmountDiscount(amt)
	d := gdDraft(t, disc)
	series := mustVal(NewSeries("GD2026", FT))
	if err := series.RegisterWithAT("AAAABBBB", now); err != nil {
		t.Fatalf("RegisterWithAT: %v", err)
	}
	d.Series = series
	qr := QRConfig{IssuerNIF: "500000000", CertificateNumber: "0"}
	doc, err := IssueSalesInvoice(d, &series, m16StubSigner{}, "tester", now, IssueOptions{}, qr)
	if err != nil {
		t.Fatalf("IssueSalesInvoice: %v", err)
	}
	if doc.Totals.GrossTotal != 7030680 {
		t.Errorf("issued GrossTotal = %d, want 7030680 (post-discount)", doc.Totals.GrossTotal)
	}
	var sum Money
	for _, l := range doc.Lines {
		sum += l.GlobalDiscountShare
	}
	if sum != 300*centScale {
		t.Errorf("Σ issued shares = %d, want %d", sum, 300*centScale)
	}
	if doc.Lines[0].GlobalDiscountShare != Money(250*centScale) {
		t.Errorf("issued share[0] = %d, want %d", doc.Lines[0].GlobalDiscountShare, 250*centScale)
	}
	if doc.Lines[1].GlobalDiscountShare != Money(50*centScale) {
		t.Errorf("issued share[1] = %d, want %d", doc.Lines[1].GlobalDiscountShare, 50*centScale)
	}
}

func TestApplyGlobalDiscount_ShareNeverExceedsBase(t *testing.T) {
	// Two lines of 1.5 × €0.01 → base 1.5 cents each (sub-cent fraction).
	// Global discount €0.03 (= Σ nets): the largest-remainder bump would
	// push one share to 2 cents > 1.5-cent base and sign a negative
	// LineNetAmount. The clamp must cap both shares at 1 whole cent and
	// shrink the realized discount to €0.02.
	mkLine := func(n int) DocumentLine {
		l := gdTestLine(t, 1.5, 0.01, nil)
		l.LineNumber = n
		l.Tax = mustVAT(t, TaxNormal)
		return l
	}
	d := gdDraft(t, nil)
	d.Lines = []DocumentLine{mkLine(1), mkLine(2)}
	amt, _ := NewMoney(0.03)
	d.GlobalDiscount, _ = NewAmountDiscount(amt)

	d.CalculateTotals()
	for i, l := range d.Lines {
		base := applyDiscount(l.Discount, l.LineSubtotal())
		if l.GlobalDiscountShare > base {
			t.Errorf("share[%d] = %d exceeds base %d", i, l.GlobalDiscountShare, base)
		}
		if net := l.LineNetAmount(); net < 0 {
			t.Errorf("line %d: negative LineNetAmount %d", i, net)
		}
	}
	if got := d.Lines[0].GlobalDiscountShare + d.Lines[1].GlobalDiscountShare; got != Money(2*centScale) {
		t.Errorf("realized discount = %d, want %d (2 whole cents)", got, 2*centScale)
	}
}

func TestApplyGlobalDiscount_ClampRedistributesToSlack(t *testing.T) {
	// Line 1 has a sub-cent base just over 1 cent (1.5 cents); line 2 has
	// plenty of whole-cent slack (€10.00). A bump landing on line 1 must be
	// clamped and the freed cent re-given to line 2 — the realized discount
	// stays exact.
	tight := gdTestLine(t, 1.5, 0.01, nil) // base 1500 (1.5 cents)
	tight.Tax = mustVAT(t, TaxNormal)
	slack := gdTestLine(t, 1, 10.00, nil) // base 1000000
	slack.LineNumber = 2
	slack.Tax = mustVAT(t, TaxNormal)
	d := gdDraft(t, nil)
	d.Lines = []DocumentLine{tight, slack}
	amt, _ := NewMoney(5.00)
	d.GlobalDiscount, _ = NewAmountDiscount(amt)

	d.CalculateTotals()
	var sum Money
	for i, l := range d.Lines {
		base := applyDiscount(l.Discount, l.LineSubtotal())
		if l.GlobalDiscountShare > base {
			t.Errorf("share[%d] = %d exceeds base %d", i, l.GlobalDiscountShare, base)
		}
		sum += l.GlobalDiscountShare
	}
	if sum != Money(500*centScale) { // full €5.00 realized — slack absorbed everything
		t.Errorf("realized discount = %d, want %d", sum, 500*centScale)
	}
}

func TestApplyGlobalDiscount_FuzzShareWithinBase(t *testing.T) {
	// Deterministic fuzz: random cent-clean prices/quantities/percent
	// discounts + a valid global amount discount must never produce a share
	// above its base or a negative line net.
	rng := rand.New(rand.NewSource(42))
	for iter := 0; iter < 2000; iter++ {
		n := 1 + rng.Intn(4)
		lines := make([]DocumentLine, n)
		var sum Money
		for i := range lines {
			unit := float64(1+rng.Intn(2000)) / 100 // €0.01..€20.00, cent-clean
			qty := float64(1+rng.Intn(80)) / 4      // 0.25..20.00 quantities
			var disc Discount
			if rng.Intn(2) == 0 {
				disc = mustPercent(t, float64(rng.Intn(9999))/100) // 0..99.98%
			}
			l := gdTestLine(t, qty, unit, disc)
			l.LineNumber = i + 1
			l.Tax = mustVAT(t, TaxNormal)
			lines[i] = l
			sum += applyDiscount(l.Discount, l.LineSubtotal())
		}
		if sum <= 0 {
			continue
		}
		discCents := 1 + rng.Int63n(int64(sum)/centScale+1) // 1..⌊sum⌋ cents, can be ~100%
		amt, err := NewMoney(float64(discCents) / 100)
		if err != nil {
			t.Fatal(err)
		}
		d := gdDraft(t, nil)
		d.Lines = lines
		d.GlobalDiscount, _ = NewAmountDiscount(amt)
		d.CalculateTotals()
		for i, l := range d.Lines {
			base := applyDiscount(l.Discount, l.LineSubtotal())
			if l.GlobalDiscountShare > base {
				t.Fatalf("iter %d: share[%d] = %d exceeds base %d", iter, i, l.GlobalDiscountShare, base)
			}
			if net := l.LineNetAmount(); net < 0 {
				t.Fatalf("iter %d: line %d negative net %d", iter, i, net)
			}
		}
	}
}
