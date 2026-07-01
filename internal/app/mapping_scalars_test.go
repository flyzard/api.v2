package app

import "testing"

func TestRateFromMicroRoundTrips(t *testing.T) {
	r, err := rateFromMicro(1085000)
	if err != nil {
		t.Fatalf("rateFromMicro: %v", err)
	}
	if got := int64(r); got != 1085000 { // domain.ExchangeRate is an int64 at 1e6 scale
		t.Fatalf("rate micro = %d, want 1085000", got)
	}
}

func TestLisbonDateParses(t *testing.T) {
	if _, err := lisbonDate("2026-05-08"); err != nil {
		t.Fatalf("lisbonDate: %v", err)
	}
	if _, err := lisbonDate("not-a-date"); err == nil {
		t.Fatal("malformed date must error KindInvalid")
	}
}
