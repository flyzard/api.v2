package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func mustMoney(t *testing.T, v float64) Money {
	t.Helper()
	m, err := NewMoney(v)
	if err != nil {
		t.Fatalf("NewMoney(%v): %v", v, err)
	}
	return m
}

func mustQuantity(t *testing.T, v float64) Quantity {
	t.Helper()
	q, err := NewQuantity(v)
	if err != nil {
		t.Fatalf("NewQuantity(%v): %v", v, err)
	}
	return q
}

func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic")
		}
	}()
	fn()
}

func mustPercentDiscount(t *testing.T, v float64) Discount {
	t.Helper()
	d, err := NewPercentDiscount(v)
	if err != nil {
		t.Fatalf("NewPercentDiscount(%v): %v", v, err)
	}
	return d
}

func TestMoneyString(t *testing.T) {
	m := mustMoney(t, 1.5)
	if got, want := m.String(), "€1.50000"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestMoneyFormat2DP(t *testing.T) {
	// Cent-precise inputs go through NewMoney.
	centCases := []struct {
		euros float64
		want  string
	}{
		{0, "0.00"},
		{1.5, "1.50"},
		{123.45, "123.45"},
	}
	for _, tc := range centCases {
		m := mustMoney(t, tc.euros)
		if got := m.Format2DP(); got != tc.want {
			t.Errorf("Money(%v).Format2DP() = %q, want %q", tc.euros, got, tc.want)
		}
	}
	// Sub-cent rounding edges. NewMoney refuses these now (Policy B), but
	// Mul/MulPercent can still produce sub-cent intermediates, so Format2DP
	// must still round them correctly. Construct via Money literal.
	subCentCases := []struct {
		scaled int64
		want   string
	}{
		{400, "0.00"},  // €0.004 → rounds down
		{500, "0.01"},  // €0.005 → half-away rounds up
		{1500, "0.02"}, // €0.015 → half-away
		{9_999_500, "100.00"},
		{100_099_900, "1001.00"},
		{-12_345, "-0.12"},
	}
	for _, tc := range subCentCases {
		if got := Money(tc.scaled).Format2DP(); got != tc.want {
			t.Errorf("Money(%d).Format2DP() = %q, want %q", tc.scaled, got, tc.want)
		}
	}
}

func TestNewMoney_RejectsSubCent(t *testing.T) {
	cases := []float64{0.005, 0.001, 1.234, 99.999}
	for _, v := range cases {
		if _, err := NewMoney(v); !errors.Is(err, ErrSubCentPrecision) {
			t.Errorf("NewMoney(%v): want ErrSubCentPrecision, got %v", v, err)
		}
	}
}

func TestMoney_MarshalCents(t *testing.T) {
	m := mustMoney(t, 49.50)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "4950" {
		t.Errorf("MarshalJSON = %s, want 4950", b)
	}
	var back Money
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back != m {
		t.Errorf("round-trip: got %d, want %d", back, m)
	}
}

func TestMoney_MarshalNegativeCents(t *testing.T) {
	m := Money(-12_345_000) // -€123.45
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "-12345" {
		t.Errorf("MarshalJSON = %s, want -12345", b)
	}
}

func TestMoneyAdd(t *testing.T) {
	got := mustMoney(t, 0.1).Add(mustMoney(t, 0.2))
	want := mustMoney(t, 0.3)
	if got != want {
		t.Errorf("0.1 + 0.2 = %v, want %v", got, want)
	}
}

func TestNewQuantityRejectsNegative(t *testing.T) {
	if _, err := NewQuantity(-1); err == nil {
		t.Errorf("NewQuantity(-1) expected error")
	}
}

func TestDiscountNoneIsNoOp(t *testing.T) {
	base := mustMoney(t, 100)
	if got := applyDiscount(nil, base); got != base {
		t.Errorf("applyDiscount(nil, €100) = %v, want %v", got, base)
	}
}

func TestPercentDiscount(t *testing.T) {
	base := mustMoney(t, 100)
	got := mustPercentDiscount(t, 10).Apply(base)
	want := mustMoney(t, 90)
	if got != want {
		t.Errorf("10%% off €100 = %v, want %v", got, want)
	}
}

func TestAmountDiscountCaps(t *testing.T) {
	base := mustMoney(t, 50)
	d, err := NewAmountDiscount(mustMoney(t, 100))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Apply(base); got != 0 {
		t.Errorf("€100 amount off €50 = %v, want 0", got)
	}
}

func TestMulPercentRoundsHalfAway(t *testing.T) {
	p, err := NewPercent(50)
	if err != nil {
		t.Fatal(err)
	}
	// Money(1) * 50% = 0.5 → should round to 1, not truncate to 0.
	if got := Money(1).MulPercent(p); got != 1 {
		t.Errorf("Money(1).MulPercent(50%%) = %d, want 1", got)
	}
}

func TestDiscountUnmarshalRejectsExcessPercent(t *testing.T) {
	if _, err := unmarshalDiscount([]byte(`{"type":"percent","percent":99999}`)); err == nil {
		t.Errorf("expected error for percent > 100")
	}
}

func TestQuantityUnmarshalRejectsNegative(t *testing.T) {
	var q Quantity
	err := json.Unmarshal([]byte(`-5`), &q)
	if err == nil {
		t.Errorf("expected error for negative quantity")
	}
}

func TestTaxRateUnmarshalRederivesValue(t *testing.T) {
	var r TaxRate
	if err := json.Unmarshal([]byte(`{"region":"PT","category":"NOR","value":99}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Value != 2300 {
		t.Errorf("expected canonical 2300, got %d", r.Value)
	}
}

func TestTaxRateUnmarshalRejectsBadRegion(t *testing.T) {
	var r TaxRate
	if err := json.Unmarshal([]byte(`{"region":"XX","category":"NOR"}`), &r); err == nil {
		t.Errorf("expected error for unknown region")
	}
}

func TestTaxRateUnmarshalRejectsExemptionWithoutISE(t *testing.T) {
	var r TaxRate
	if err := json.Unmarshal([]byte(`{"region":"PT","category":"NOR","exemption":"M01"}`), &r); err == nil {
		t.Errorf("expected error for exemption with non-ISE category")
	}
}

func TestTaxRateValidateRejectsMismatchedValue(t *testing.T) {
	r := TaxRate{Region: PT, Category: TaxNormal, Value: 9999}
	if err := r.Validate(); err == nil {
		t.Errorf("expected mismatch error")
	}
}

func TestDocumentLineUnmarshalRejectsNegativePrice(t *testing.T) {
	var l DocumentLine
	err := json.Unmarshal([]byte(`{"unit_price":-1,"quantity":1,"tax_rate":{"region":"PT","category":"NOR"}}`), &l)
	if err == nil {
		t.Errorf("expected error for negative unit price")
	}
}

func TestDocumentLineUnmarshalRejectsZeroQuantity(t *testing.T) {
	var l DocumentLine
	err := json.Unmarshal([]byte(`{"unit_price":10,"quantity":0,"tax_rate":{"region":"PT","category":"NOR"}}`), &l)
	if err == nil {
		t.Errorf("expected error for zero quantity")
	}
}

func TestNewMoneyRejectsOverflow(t *testing.T) {
	if _, err := NewMoney(1e15); err == nil {
		t.Errorf("expected overflow error for 1e15 euros")
	}
}

func TestMulPercentPanicsOnOutOfRangePercent(t *testing.T) {
	mustPanic(t, func() { Money(100).MulPercent(Percent(20000)) })
}

func TestMoney_MulOverflowDetected(t *testing.T) {
	// Both operands near the int64 ceiling — product would overflow.
	mustPanic(t, func() { Money(1 << 40).Mul(Quantity(1 << 30)) })
}

func TestNewPercentDiscountReturnsNilOnError(t *testing.T) {
	d, err := NewPercentDiscount(200)
	if err == nil {
		t.Errorf("expected error for percent > 100")
	}
	if d != nil {
		t.Errorf("expected nil Discount on error, got %+v", d)
	}
}

func TestLineTotal(t *testing.T) {
	tax, err := NewVATLineTax(PT, TaxNormal, "", "")
	if err != nil {
		t.Fatal(err)
	}
	line := DocumentLine{
		UnitPrice: mustMoney(t, 100),
		Quantity:  mustQuantity(t, 1),
		Discount:  mustPercentDiscount(t, 10),
		Tax:       tax,
	}
	got := line.LineTotal()
	want := mustMoney(t, 110.70)
	if got != want {
		t.Errorf("LineTotal = %v, want %v", got, want)
	}
}
