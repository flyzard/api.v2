package domain

import "testing"

func TestMoneyCents(t *testing.T) {
	// scale = 100_000, centScale = 1_000. Cents() rounds half away from zero.
	cases := []struct {
		in   Money
		want int64
	}{
		{Money(1250), 1},         // 0.01250 -> 0.01
		{Money(1500), 2},         // 0.01500 -> 0.02 (half away)
		{Money(1538), 2},         // 0.01538 -> 0.02
		{Money(12300000), 12300}, // 123.00
		{Money(-1500), -2},       // -0.015 -> -0.02
	}
	for _, c := range cases {
		if got := c.in.Cents(); got != c.want {
			t.Errorf("Money(%d).Cents() = %d, want %d", int64(c.in), got, c.want)
		}
	}
}

func TestMoneyFromCents(t *testing.T) {
	if got := MoneyFromCents(1).Format2DP(); got != "0.01" {
		t.Errorf("MoneyFromCents(1).Format2DP() = %q, want 0.01", got)
	}
	if got := MoneyFromCents(13346).Format2DP(); got != "133.46" {
		t.Errorf("MoneyFromCents(13346).Format2DP() = %q, want 133.46", got)
	}
	// Round-trip: Cents then FromCents yields the same 2dp rendering.
	if got := MoneyFromCents(Money(1538).Cents()).Format2DP(); got != "0.02" {
		t.Errorf("round-trip = %q, want 0.02", got)
	}
}
