package at

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"
)

const (
	defaultMaxRetries     = 3
	defaultInitialBackoff = 500 * time.Millisecond
	defaultMaxBackoff     = 10 * time.Second
	defaultOpTimeout      = 30 * time.Second
)

// RetrySettings tunes the exponential backoff applied to transient AT errors.
// Zero values fall back to the package defaults.
type RetrySettings struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func (rs RetrySettings) withDefaults() RetrySettings {
	if rs.MaxRetries <= 0 {
		rs.MaxRetries = defaultMaxRetries
	}
	if rs.InitialBackoff <= 0 {
		rs.InitialBackoff = defaultInitialBackoff
	}
	if rs.MaxBackoff <= 0 {
		rs.MaxBackoff = defaultMaxBackoff
	}
	return rs
}

// retryable executes fn with exponential backoff for transient Errors.
func retryable[T any](ctx context.Context, logger *slog.Logger, rs RetrySettings, op string, fn func() (T, error)) (T, error) {
	rs = rs.withDefaults()
	backoff := rs.InitialBackoff

	var lastErr error
	for attempt := range rs.MaxRetries {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		var atErr Error
		if !errors.As(err, &atErr) || !atErr.IsRetryable() {
			return result, err
		}

		lastErr = err

		if attempt == rs.MaxRetries-1 {
			break
		}

		logger.InfoContext(ctx, "AT retrying transient error",
			slog.String("operation", op),
			slog.Int("attempt", attempt+1),
			slog.Int("max_retries", rs.MaxRetries),
			slog.String("error_code", atErr.Code),
			slog.Duration("backoff", backoff),
		)

		if err := sleepCtx(ctx, jitteredBackoff(backoff)); err != nil {
			var zero T
			return zero, err
		}

		backoff = min(backoff*2, rs.MaxBackoff)
	}

	var zero T
	return zero, lastErr
}

// retryableNoResult is retryable for functions that return only error.
func retryableNoResult(ctx context.Context, logger *slog.Logger, rs RetrySettings, op string, fn func() error) error {
	_, err := retryable(ctx, logger, rs, op, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// jitteredBackoff returns a duration between backoff/2 and backoff.
func jitteredBackoff(backoff time.Duration) time.Duration {
	half := backoff / 2
	jitter := time.Duration(rand.Int64N(int64(half) + 1)) //nolint:gosec // jitter doesn't need crypto randomness
	return half + jitter
}

// sleepCtx sleeps for d, returning early if ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("sleep interrupted: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}
