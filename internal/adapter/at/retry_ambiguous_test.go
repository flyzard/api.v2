package at

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

func TestRetryable_FlagsAmbiguousAfterTransient(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	calls := 0
	_, err := retryable(context.Background(), logger, fastRetry(), "op", func() (int, error) {
		calls++
		if calls == 1 {
			return 0, Error{Code: "HTTP_502", Message: "bad gateway"}
		}
		return 0, Error{Code: "4001", Message: "Documento ja registado"}
	})
	var atErr Error
	if !errors.As(err, &atErr) || !atErr.Ambiguous {
		t.Fatalf("deterministic failure after a transient one must be Ambiguous, got %v", err)
	}
}

func TestRetryable_FirstAttemptFailureIsNotAmbiguous(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	_, err := retryable(context.Background(), logger, fastRetry(), "op", func() (int, error) {
		return 0, Error{Code: "4001", Message: "rejected"}
	})
	var atErr Error
	if errors.As(err, &atErr) && atErr.Ambiguous {
		t.Fatal("clean first-attempt rejection wrongly marked ambiguous")
	}
}
