package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCurrencyCodeValid(t *testing.T) {
	cases := map[string]bool{
		"USD": true,
		"JPY": true,
		"GBP": true,
		"EUR": false, // EUR is excluded by XSD; SAF-T Currency block only appears for non-EUR
		"XYZ": false,
		"":    false,
		"usd": false,
	}
	for in, want := range cases {
		if got := CurrencyCode(in).IsValid(); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestExchangeRate(t *testing.T) {
	// Strong currency (sub-1)
	r, err := NewExchangeRate(0.92)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Float64(); got < 0.91 || got > 0.93 {
		t.Errorf("USD/EUR round-trip: got %v", got)
	}
	// Weak currency (>>1)
	r, err = NewExchangeRate(170.5)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Float64(); got < 170.0 || got > 171.0 {
		t.Errorf("JPY/EUR round-trip: got %v", got)
	}
	// Reject zero and negative
	for _, v := range []float64{0, -1, -0.5} {
		if _, err := NewExchangeRate(v); err == nil {
			t.Errorf("rate %v: expected error", v)
		}
	}
}

func TestCurrencyJSON(t *testing.T) {
	rate, _ := NewExchangeRate(1.085)
	amt, _ := NewMoney(100.0)
	c, err := NewCurrency("USD", amt, rate, time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back Currency
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Code != "USD" {
		t.Errorf("code: got %q want USD", back.Code)
	}
}

func TestCurrencyValidateRejectsZeroAmount(t *testing.T) {
	rate, _ := NewExchangeRate(1.0)
	if _, err := NewCurrency("USD", 0, rate, time.Now()); err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestCurrencyValidateRejectsZeroDate(t *testing.T) {
	rate, _ := NewExchangeRate(1.0)
	amt, _ := NewMoney(100)
	if _, err := NewCurrency("USD", amt, rate, time.Time{}); err == nil {
		t.Fatal("expected error for zero currency date")
	}
}
