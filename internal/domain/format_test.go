package domain

import (
	"encoding/json"
	"testing"
)

// TestMoneyMarshalJSONRoundsLikeFormat2DP pins MarshalJSON to the same
// half-away-from-zero cent rounding Format2DP uses for the canonical string
// and QR. Sub-cent totals (e.g. €33.33 × 23% VAT = 766590 scaled = 766.59¢)
// must persist as the value that is signed and printed, not truncate.
func TestMoneyMarshalJSONRoundsLikeFormat2DP(t *testing.T) {
	cases := []struct {
		scaled Money
		want   string
	}{
		{766590, "767"},   // 766.59¢ rounds up, truncation gave 766
		{766490, "766"},   // below half rounds down
		{-766590, "-767"}, // half-away-from-zero on negatives
		{4950000, "4950"}, // whole cents unchanged (€49.50)
	}
	for _, c := range cases {
		got, err := json.Marshal(c.scaled)
		if err != nil {
			t.Fatalf("Marshal(%d): %v", c.scaled, err)
		}
		if string(got) != c.want {
			t.Errorf("Marshal(%d) = %s, want %s (Format2DP=%s)", c.scaled, got, c.want, c.scaled.Format2DP())
		}
	}
}

func TestHashFourChars(t *testing.T) {
	// 40-char synthetic hash: positions 1,11,21,31 hold A,B,C,D.
	h := Hash("Axxxxxxxxx" + "Bxxxxxxxxx" + "Cxxxxxxxxx" + "Dxxxxxxxxx")
	if got := h.FourChars(); got != "ABCD" {
		t.Errorf("FourChars = %q, want ABCD", got)
	}
	if got := Hash("AB").FourChars(); got != "A" {
		t.Errorf("short hash FourChars = %q, want A (bounds-guarded)", got)
	}
	if got := Hash("").FourChars(); got != "" {
		t.Errorf("empty hash FourChars = %q, want empty string", got)
	}
}

func TestQuantityString(t *testing.T) {
	q, err := NewQuantity(2.5)
	if err != nil {
		t.Fatalf("NewQuantity: %v", err)
	}
	if got := q.String(); got != "2.5" {
		t.Errorf("Quantity.String = %q, want 2.5", got)
	}
	q, err = NewQuantity(2)
	if err != nil {
		t.Fatalf("NewQuantity(2): %v", err)
	}
	if got := q.String(); got != "2" {
		t.Errorf("Quantity.String (integer) = %q, want 2 (prec=-1 trimming)", got)
	}
}

func TestPercentFormat2DP(t *testing.T) {
	p, err := NewPercent(23)
	if err != nil {
		t.Fatalf("NewPercent: %v", err)
	}
	if got := p.Format2DP(); got != "23.00" {
		t.Errorf("Format2DP = %q, want 23.00", got)
	}
	p, err = NewPercent(6.5)
	if err != nil {
		t.Fatalf("NewPercent(6.5): %v", err)
	}
	if got := p.Format2DP(); got != "6.50" {
		t.Errorf("Format2DP (fractional) = %q, want 6.50", got)
	}
}
