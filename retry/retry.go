// Package retry provides a small bounded-exponential-jitter retry helper
// for PostgreSQL serialization (40001) and deadlock (40P01) failures.
//
// It does NOT retry on validation, authorization, or invariant failures —
// those are permanent and must bubble up to the caller (Phase 2 §
// "Serializable retry helper").
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// MaxAttempts is the default upper bound for serialization retries.
const MaxAttempts = 5

// BaseBackoff is the initial delay between retries; subsequent delays double.
const BaseBackoff = 25 * time.Millisecond

// IsRetryable reports whether err looks like a PostgreSQL serialization or
// deadlock failure. We also retry on SQLSTATE 40001, 40P01, and on
// 55P03 (lock not available).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01", "55P03":
			return true
		}
		return false
	}
	// Some Go drivers wrap the SQL state in a string.
	msg := err.Error()
	return strings.Contains(msg, "could not serialize access") ||
		strings.Contains(msg, "deadlock detected")
}

// Do executes fn with bounded retries on serialization failures. It returns
// the last error if attempts are exhausted.
//
// ctx cancellation aborts the loop. fn should be a self-contained
// transactional unit of work; the helper does not wrap it in a transaction.
func Do(ctx context.Context, fn func(context.Context) error) error {
	return DoWithAttempts(ctx, MaxAttempts, fn)
}

// DoWithAttempts is Do with a custom upper bound (useful for tests).
func DoWithAttempts(ctx context.Context, attempts int, fn func(context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if !IsRetryable(lastErr) {
			return lastErr
		}
		if i == attempts-1 {
			break
		}
		sleep := backoff(i)
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("retry: exhausted %d attempts: %w", attempts, lastErr)
}

func backoff(attempt int) time.Duration {
	// 25ms * 2^attempt + jitter, capped at 1s.
	d := BaseBackoff << attempt
	if d > time.Second {
		d = time.Second
	}
	// ±50% jitter.
	j := time.Duration(rand.Int64N(int64(d)))
	return d/2 + j
}
