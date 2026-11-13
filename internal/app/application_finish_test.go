package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stopProbe is a registered stop that records the context it was handed.
type stopProbe struct {
	ran      chan struct{}
	err      error
	deadline bool
	remain   time.Duration
}

func addStopProbe(l *generationLifecycle) *stopProbe {
	probe := &stopProbe{ran: make(chan struct{})}
	l.addStop(func(ctx context.Context) error {
		probe.err = ctx.Err()
		var at time.Time
		at, probe.deadline = ctx.Deadline()
		probe.remain = time.Until(at)
		close(probe.ran)
		return nil
	})
	return probe
}

func waitStop(t *testing.T, probe *stopProbe) {
	t.Helper()
	select {
	case <-probe.ran:
	case <-time.After(2 * time.Second):
		t.Fatal("the stop never ran")
	}
}

func waitDone(t *testing.T, l *generationLifecycle) {
	t.Helper()
	select {
	case <-l.done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown never finished")
	}
}

// Stops are River's Stop, the store close, the recorder flush: they degrade to forced when handed
// a dead context. Whatever happened to the join, each runs on a live budget of its own.
func TestShutdownStopsGetALiveBudgetWhateverTheCallersContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		// ctx builds the shutdown context; unwind is how long the worker takes after cancel.
		ctx    func(t *testing.T) context.Context
		unwind time.Duration
	}{
		{"deadline passes while a worker is still unwinding", func(t *testing.T) context.Context {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
			t.Cleanup(cancel)
			return ctx
		}, 150 * time.Millisecond},
		{"context already cancelled", func(t *testing.T) context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx
		}, 80 * time.Millisecond},
		{"context already past its deadline", func(t *testing.T) context.Context {
			ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			t.Cleanup(cancel)
			return ctx
		}, 0},
		{"context live and the worker returns in time", func(t *testing.T) context.Context { return t.Context() }, 20 * time.Millisecond},
		{"context live and no worker", func(t *testing.T) context.Context { return t.Context() }, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newGenerationLifecycle(t.Context())
			l.stopBudget = time.Second
			if tc.unwind >= 0 {
				l.goRun(func(ctx context.Context) {
					<-ctx.Done()
					time.Sleep(tc.unwind)
				})
			}
			probe := addStopProbe(l)
			go func() { _ = l.shutdown(tc.ctx(t)) }()
			waitStop(t, probe)
			if probe.err != nil {
				t.Errorf("stop ran on a dead context: %v", probe.err)
			}
			if !probe.deadline || probe.remain <= 0 || probe.remain > time.Second {
				t.Errorf("stop's own budget = deadline %v, %v remaining; want a live deadline within %v", probe.deadline, probe.remain, time.Second)
			}
		})
	}
}

// One slow stop must not leave the next one nothing: the budget is per stop.
func TestShutdownEachStopHasItsOwnBudget(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 40 * time.Millisecond
	later := addStopProbe(l)
	// Stops run in reverse order, so this one, registered last, runs first and eats its budget.
	var slowErr error
	l.addStop(func(ctx context.Context) error {
		<-ctx.Done()
		slowErr = ctx.Err()
		return slowErr
	})
	err := l.shutdown(t.Context())
	waitStop(t, later)
	if later.err != nil {
		t.Errorf("the stop after a slow one ran on a dead context: %v", later.err)
	}
	if !errors.Is(slowErr, context.DeadlineExceeded) || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the slow stop = %v, shutdown = %v; want its own budget to run out and be reported", slowErr, err)
	}
}

// The caller giving up on waiting must still be reported as the shutdown's outcome, and a worker
// that outlived the deadline is named.
func TestShutdownStuckWorkerIsReportedAtTheDeadline(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = time.Second
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	l.goRun(func(context.Context) { <-release })
	probe := addStopProbe(l)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := l.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v, want deadline exceeded", err)
	}
	waitStop(t, probe)
	waitDone(t, l)
	if !strings.Contains(l.err.Error(), "workers still running") {
		t.Errorf("outcome = %v, want the stuck worker named", l.err)
	}
	if probe.err != nil {
		t.Errorf("stop ran on a dead context: %v", probe.err)
	}
}

// Once the workers have returned and the network resources are quiesced, a deadline that also
// passed is not a failure: the generation is done, and says so.
func TestShutdownReportsDoneWhenEverythingFinishedEvenIfTheDeadlinePassed(t *testing.T) {
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	expired := func() context.Context {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Minute))
		cancel()
		return ctx
	}
	for name, ctx := range map[string]func() context.Context{"cancelled": cancelled, "expired": expired} {
		t.Run(name, func(t *testing.T) {
			// Workers that return at once on cancel, settled before the dead context is used.
			for range 50 {
				l := newGenerationLifecycle(context.Background())
				l.goRun(func(ctx context.Context) { <-ctx.Done() })
				quiesced := false
				l.addQuiesce(func(context.Context) error { quiesced = true; return nil })
				probe := addStopProbe(l)
				// Settle the quiesce and the workers on a live context, so the only thing
				// left that can go wrong is the dead one handed to shutdown.
				if err := l.quiesce(context.Background()); err != nil || !quiesced {
					t.Fatalf("setup quiesce = %v, %v", err, quiesced)
				}
				l.wg.Wait()

				if err := l.shutdown(ctx()); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("shutdown = %v", err)
				}
				waitStop(t, probe)
				waitDone(t, l)
				if l.err != nil {
					t.Fatalf("outcome = %v, want none: the quiesce had finished and every worker had returned", l.err)
				}
			}
		})
	}
}

// The reported error was what shutdown returned too: with everything done by the time the caller
// looks, an expired context must not turn success into ctx.Err().
func TestShutdownAndQuiesceReturnTheirResultWhenBothTheResultAndTheDeadlineAreReady(t *testing.T) {
	l := newGenerationLifecycle(context.Background())
	l.goRun(func(ctx context.Context) { <-ctx.Done() })
	if err := l.shutdown(context.Background()); err != nil {
		t.Fatalf("setup shutdown = %v", err)
	}
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	// A select that picked at random would fail one of these about half the time.
	for range 100 {
		if err := l.shutdown(dead); err != nil {
			t.Fatalf("shutdown after completion with a dead context = %v, want the (nil) outcome", err)
		}
		if err := l.quiesce(dead); err != nil {
			t.Fatalf("quiesce after completion with a dead context = %v, want the (nil) outcome", err)
		}
	}
	// And the recorded outcome survives, not the context's.
	failing := newGenerationLifecycle(context.Background())
	stopErr := errors.New("stop failed")
	failing.addStop(func(context.Context) error { return stopErr })
	if err := failing.shutdown(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("setup = %v", err)
	}
	for range 100 {
		if err := failing.shutdown(dead); !errors.Is(err, stopErr) {
			t.Fatalf("shutdown after a failed stop with a dead context = %v, want %v", err, stopErr)
		}
	}
}

// With a quiesced generation and no worker running, a dead context must not read as a running one.
func TestWaitWorkersPrefersWorkersThatHaveReturned(t *testing.T) {
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	l := newGenerationLifecycle(context.Background())
	var ran atomic.Bool
	l.goRun(func(context.Context) { ran.Store(true) })
	l.wg.Wait()
	for range 100 {
		if err := l.waitWorkers(dead); err != nil {
			t.Fatalf("waitWorkers with every worker returned = %v, want nil", err)
		}
	}
	// The negative control: a worker that has not returned is reported.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	l = newGenerationLifecycle(context.Background())
	l.goRun(func(context.Context) { <-release })
	if err := l.waitWorkers(dead); err == nil || !strings.Contains(err.Error(), "workers still running") {
		t.Fatalf("waitWorkers with a worker running = %v, want it named", err)
	}
	// Every kind of worker counts: on-demand and interactive work too.
	l2 := newGenerationLifecycle(context.Background())
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	if !l2.goOnDemand(func(context.Context) { <-hold }) {
		t.Fatal("on-demand refused")
	}
	if err := l2.waitWorkers(dead); err == nil {
		t.Error("an on-demand worker still running was not reported")
	}
	l3 := newGenerationLifecycle(context.Background())
	if err := l3.startInteractiveOperation(0, func(context.Context) error { <-hold; return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if err := l3.waitWorkers(dead); err == nil {
		t.Error("an interactive operation still running was not reported")
	}
}
