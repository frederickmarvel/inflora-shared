// Package runtime contains cross-service helpers for signal cancellation,
// bounded startup timeouts, readiness probes, and graceful shutdown coordination.
//
// Every Inflora production binary uses this package to:
//   - Cancel a context on SIGINT / SIGTERM.
//   - Drain workers and listeners within a configurable deadline.
//   - Run bounded startup probes (DB ping, NATS connect, gRPC dial).
package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ErrStartupTimeout indicates a dependency probe did not complete within the
// configured startup timeout.
var ErrStartupTimeout = errors.New("runtime: startup timeout")

// WaitForSignal blocks until SIGINT or SIGTERM arrives or ctx is done.
// It returns nil when a signal was received, ctx.Err() otherwise.
func WaitForSignal(ctx context.Context) error {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(ch)
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// LivenessHandler is a function that returns true when the service is ready
// to accept traffic. It is called by Readiness periodically.
type LivenessHandler func(context.Context) (ready bool, err error)

// Run starts the worker set and blocks until the parent is cancelled, a
// process signal arrives, or a worker fails. A worker failure immediately
// cancels sibling workers. Workers then have up to shutdownTimeout to drain,
// after which Run returns the triggering error or a shutdown-timeout error.
//
//	startCtx — startup deadline for dependency probes
//	shutdownTimeout — drain deadline after cancellation
//	workers — at least one must be provided
func Run(parent context.Context, startTimeout, shutdownTimeout time.Duration, probe LivenessHandler, workers ...Worker) error {
	if len(workers) == 0 {
		return errors.New("runtime: at least one worker is required")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	if probe != nil {
		if err := probeUntil(ctx, probe, startTimeout); err != nil {
			return fmt.Errorf("runtime: startup probe failed: %w", err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(workers))
	for _, w := range workers {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errs <- fmt.Errorf("runtime: worker %q: %w", w.Name(), err)
			}
		}()
	}

	stop := make(chan error, 1)
	go func() { stop <- WaitForSignal(ctx) }()
	var workerErr error
	var stopErr error
	select {
	case workerErr = <-errs:
		cancel() // a failed worker stops all siblings immediately
	case stopErr = <-stop:
		cancel()
	}
	// WaitForSignal returns ctx.Err() when the parent context was cancelled
	// rather than a real signal. We treat that as a clean shutdown.
	cancelSignal := stopErr == nil || errors.Is(stopErr, context.Canceled)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	if shutdownTimeout <= 0 {
		<-done
	} else {
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			return fmt.Errorf("runtime: shutdown timeout after %s", shutdownTimeout)
		}
	}
	close(errs)
	if workerErr != nil {
		return workerErr
	}
	if !cancelSignal && stopErr != nil {
		return stopErr
	}
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func probeUntil(ctx context.Context, probe LivenessHandler, timeout time.Duration) error {
	if timeout <= 0 {
		ready, err := probe(ctx)
		if err != nil {
			return err
		}
		if !ready {
			return ErrStartupTimeout
		}
		return nil
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	ready, err := probe(ctx)
	if err != nil {
		return err
	}
	if ready {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("%w: after %s", ErrStartupTimeout, timeout)
			}
			ready, err := probe(ctx)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

// Worker is the unit of work managed by Run.
type Worker interface {
	Name() string
	Run(context.Context) error
}

// WorkerFunc adapts a named closure to the Worker interface.
type WorkerFunc struct {
	N string
	F func(context.Context) error
}

// Name returns the worker name.
func (w *WorkerFunc) Name() string { return w.N }

// Run executes the worker function.
func (w *WorkerFunc) Run(ctx context.Context) error { return w.F(ctx) }
