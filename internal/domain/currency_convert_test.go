package domain

import (
	"testing"
	"time"
)

func TestCurrencyConvert(t *testing.T) {
	c := Currency{
		Code:         "USD",
		Amount:       mustVal(NewMoney(100.00)),
		ExchangeRate: mustVal(NewExchangeRate(1.085)),
		Date:         time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC),
	}
	// NativeAmount is exactly Convert(Amount).
	if c.Convert(c.Amount) != c.NativeAmount() {
		t.Errorf("Convert(Amount) %d != NativeAmount() %d", int64(c.Convert(c.Amount)), int64(c.NativeAmount()))
	}
	// Converting the document gross expresses it in the original currency.
	if got := c.Convert(mustVal(NewMoney(123.00))).Format2DP(); got != "133.46" {
		t.Errorf("Convert(123.00).Format2DP() = %q, want 133.46", got)
	}
}
