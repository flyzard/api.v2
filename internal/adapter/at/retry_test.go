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
		return "", Error{Code: "TIMEOUT", Message: "t/o"}
	})
	if err == nil {
		t.Fatal("want error after exhausting retries")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryableNoResult(t *testing.T) {
	calls := 0
	err := retryableNoResult(context.Background(), slog.Default(), fastRetry(), "op", func() error {
		calls++
		if calls < 2 {
			return Error{Code: "CONNECTION", Message: "refused"}
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
}
