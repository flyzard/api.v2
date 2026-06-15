package at

import (
	"context"
	"testing"
	"time"
)

func TestEnsureDeadline_CoversRetryBudget(t *testing.T) {
	c, err := NewClient(Config{
		TaxpayerNIF: "123456789", Username: "u", Password: "p",
		SeriesURL: "https://example.invalid/series",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := c.ensureDeadline(context.Background())
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("no deadline applied")
	}
	// 3 attempts × 30s request timeout + backoff must fit inside the budget;
	// the old default (30s) allowed exactly one slow failure and zero retries.
	if remaining := time.Until(dl); remaining < 90*time.Second {
		t.Fatalf("operation budget %v cannot cover MaxRetries×Timeout", remaining)
	}
}
