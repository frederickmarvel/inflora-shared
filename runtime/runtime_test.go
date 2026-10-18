package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForSignalReturnsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WaitForSignal(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunRequiresWorker(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), 0, 0, nil); err == nil {
		t.Fatalf("expected error with no workers")
	}
}

func TestRunDrainsWorkers(t *testing.T) {
	t.Parallel()
	var ran int32
	w := &WorkerFunc{N: "ok", F: func(ctx context.Context) error {
		atomic.StoreInt32(&ran, 1)
		<-ctx.Done()
		return ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, 0, time.Second, nil, w) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&ran) != 1 {
		t.Fatalf("worker did not run")
	}
}

func TestRunReturnsWorkerFailureAndCancelsSiblings(t *testing.T) {
	t.Parallel()
	want := errors.New("worker failed")
	var siblingCanceled int32
	failed := &WorkerFunc{N: "failed", F: func(context.Context) error { return want }}
	sibling := &WorkerFunc{N: "sibling", F: func(ctx context.Context) error {
		<-ctx.Done()
		atomic.StoreInt32(&siblingCanceled, 1)
		return ctx.Err()
	}}
	start := time.Now()
	err := Run(context.Background(), 0, time.Second, nil, failed, sibling)
	if !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want %v", err, want)
	}
	if time.Since(start) > time.Second {
		t.Fatal("Run did not return promptly")
	}
	if atomic.LoadInt32(&siblingCanceled) != 1 {
		t.Fatal("sibling was not canceled")
	}
}

func TestProbeRespectsTimeout(t *testing.T) {
	t.Parallel()
	err := probeUntil(context.Background(), func(context.Context) (bool, error) {
		return false, nil
	}, 50*time.Millisecond)
	if !errors.Is(err, ErrStartupTimeout) {
		t.Fatalf("err = %v, want ErrStartupTimeout", err)
	}
}

func TestProbeAcceptsReady(t *testing.T) {
	t.Parallel()
	if err := probeUntil(context.Background(), func(context.Context) (bool, error) {
		return true, nil
	}, time.Second); err != nil {
		t.Fatalf("probeUntil: %v", err)
	}
}

func TestProbePropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("probe boom")
	err := probeUntil(context.Background(), func(context.Context) (bool, error) {
		return false, wantErr
	}, time.Second)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
