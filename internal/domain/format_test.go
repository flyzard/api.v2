package domain

import "testing"

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
