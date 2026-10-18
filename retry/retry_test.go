package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryablePgSQLStates(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"40001": true,  // serialization_failure
		"40P01": true,  // deadlock_detected
		"55P03": true,  // lock_not_available
		"23505": false, // unique_violation
		"23502": false, // not_null_violation
	}
	for code, want := range cases {
		got := IsRetryable(&pgconn.PgError{Code: code})
		if got != want {
			t.Fatalf("IsRetryable(%s) = %v, want %v", code, got, want)
		}
	}
}

func TestIsRetryableIgnoresPlainErrors(t *testing.T) {
	t.Parallel()
	if IsRetryable(errors.New("boom")) {
		t.Fatalf("plain error must not be retryable")
	}
	if IsRetryable(nil) {
		t.Fatalf("nil must not be retryable")
	}
}

func TestDoReturnsNonRetryableImmediately(t *testing.T) {
	t.Parallel()
	calls := 0
	err := Do(context.Background(), func(context.Context) error {
		calls++
		return errors.New("permanent")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestDoRetriesUntilSuccess(t *testing.T) {
	t.Parallel()
	calls := 0
	err := Do(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	t.Parallel()
	calls := 0
	err := DoWithAttempts(context.Background(), 3, func(context.Context) error {
		calls++
		return &pgconn.PgError{Code: "40001"}
	})
	if err == nil {
		t.Fatalf("expected exhaustion error")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestDoHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := Do(ctx, func(context.Context) error {
		calls++
		return &pgconn.PgError{Code: "40001"}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls > 1 {
		t.Fatalf("calls = %d, want at most 1", calls)
	}
}
