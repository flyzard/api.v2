package at

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func fastRetry() RetrySettings {
	return RetrySettings{MaxRetries: 3, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}
}

// TestIsRetryable pins the retryable set to the codes the client actually
// produces: "CONNECTION" (sendSOAPRequest wraps every transport/timeout error)
// and "HTTP_<status>" for non-200 responses. "TIMEOUT"/"UNAVAILABLE"/"BUSY"
// were never emitted by any code path, and "9999" had no primary-source AT
// mapping — dead entries that misled readers about what actually retries.
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"CONNECTION", true},
		{"HTTP_502", true},
		{"HTTP_503", true},
		{"HTTP_504", true},
		{"HTTP_500", false}, // AT app-level fault, not transient
		{"TIMEOUT", false},  // never produced; timeouts arrive as CONNECTION
		{"UNAVAILABLE", false},
		{"BUSY", false},
		{"9999", false},
		{"3001", false},
	}
	for _, c := range cases {
		if got := (Error{Code: c.code}).IsRetryable(); got != c.want {
			t.Errorf("IsRetryable(%q) = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestRetryableSucceedsAfterTransientErrors(t *testing.T) {
	calls := 0
	got, err := retryable(context.Background(), slog.Default(), fastRetry(), "op", func() (string, error) {
		calls++
		if calls < 3 {
			return "", Error{Code: "HTTP_503", Message: "busy"}
		}
		return "ok", nil
	})
	if err != nil || got != "ok" {
		t.Fatalf("got %q, %v", got, err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryableStopsOnPermanentError(t *testing.T) {
	calls := 0
	_, err := retryable(context.Background(), slog.Default(), fastRetry(), "op", func() (string, error) {
		calls++
		return "", Error{Code: "3001", Message: "invalid series"}
	})
	var atErr Error
	if !errors.As(err, &atErr) || atErr.Code != "3001" {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on permanent error)", calls)
	}
}

func TestRetryableExhaustsRetries(t *testing.T) {
	calls := 0
	_, err := retryable(context.Background(), slog.Default(), fastRetry(), "op", func() (string, error) {
		calls++
		return "", Error{Code: "CONNECTION", Message: "t/o"}
	})
	if err == nil {
		t.Fatal("want error after exhausting retries")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryableRecoversAfterConnectionError(t *testing.T) {
	calls := 0
	_, err := retryable(context.Background(), slog.Default(), fastRetry(), "op", func() (struct{}, error) {
		calls++
		if calls < 2 {
			return struct{}{}, Error{Code: "CONNECTION", Message: "refused"}
		}
		return struct{}{}, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}
