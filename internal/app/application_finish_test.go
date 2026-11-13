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

// The first stop to run gets the whole shared budget live, however long the workers' join took.
func TestShutdownFirstStopGetsTheWholeSharedBudget(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = time.Second
	first := addStopProbe(l)
	if err := l.shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitStop(t, first)
	if first.err != nil || first.remain < 500*time.Millisecond {
		t.Errorf("first stop: ctx err %v, %v of the budget left, want it live and nearly whole", first.err, first.remain)
	}
}

// The caller giving up on waiting must still be reported as the shutdown's outcome, and a worker
// that outlived the deadline is named.
func TestShutdownStuckWorkerIsReportedAtTheDeadline(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 100 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	l.goRun(func(context.Context) { <-release })
	probe := addStopProbe(l)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := l.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v, want deadline exceeded", err)
	}
	// The stuck worker was waited for the whole late budget before the stop went beside it.
	if took := time.Since(start); took < 100*time.Millisecond {
		t.Errorf("shutdown returned after %v, before the late-worker budget ran out", took)
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

// Shutdown is the barrier the successor generation, the store close and the database cutover stand
// behind: when it returns, every stop has finished, and none began while a worker still ran. The
// table is every way the caller's context and the stops can relate to the workers.
func TestShutdownReturnsOnlyAfterItsStopsHaveFinishedAndNoneStartedBesideAWorker(t *testing.T) {
	cancelled := func(t *testing.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	expired := func(t *testing.T) context.Context {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}
	short := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
		t.Cleanup(cancel)
		return ctx
	}
	for _, tc := range []struct {
		name   string
		ctx    func(t *testing.T) context.Context
		unwind time.Duration // how long the worker takes after cancel; negative = no worker
		stops  int
	}{
		{"deadline passes while a worker unwinds, one stop", short, 150 * time.Millisecond, 1},
		{"deadline passes while a worker unwinds, three stops", short, 150 * time.Millisecond, 3},
		{"context already cancelled", cancelled, 100 * time.Millisecond, 2},
		{"context already past its deadline", expired, 100 * time.Millisecond, 2},
		{"live context, worker returns in time", func(t *testing.T) context.Context { return t.Context() }, 20 * time.Millisecond, 2},
		{"live context, no worker", func(t *testing.T) context.Context { return t.Context() }, -1, 2},
		{"dead context, no worker", expired, -1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newGenerationLifecycle(t.Context())
			l.stopBudget = 2 * time.Second
			var workerRunning, stopsRunning, stopsFinished atomic.Int64
			var overlap atomic.Bool
			if tc.unwind >= 0 {
				workerRunning.Store(1)
				l.goRun(func(ctx context.Context) {
					<-ctx.Done()
					time.Sleep(tc.unwind)
					workerRunning.Store(0)
				})
			}
			for range tc.stops {
				l.addStop(func(context.Context) error {
					stopsRunning.Add(1)
					if workerRunning.Load() != 0 {
						overlap.Store(true)
					}
					time.Sleep(60 * time.Millisecond)
					stopsRunning.Add(-1)
					stopsFinished.Add(1)
					return nil
				})
			}
			_ = l.shutdown(tc.ctx(t))
			if got := stopsFinished.Load(); got != int64(tc.stops) {
				t.Errorf("%d of %d stops had finished when shutdown returned", got, tc.stops)
			}
			if stopsRunning.Load() != 0 {
				t.Error("a stop was still running when shutdown returned")
			}
			if overlap.Load() {
				t.Error("a stop started while a worker still ran")
			}
			// Every later caller is held to the same barrier, not handed the outcome early.
			_ = l.shutdown(expired(t))
		})
	}
}

// The shared budget bounds the whole stop phase: a slow stop spends it, the rest still run (forced,
// on the spent context) and shutdown comes back at the budget instead of stacking one per stop.
func TestShutdownStopsShareOneBudget(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 80 * time.Millisecond
	l.stopGrace = 40 * time.Millisecond
	var ran atomic.Int64
	var secondCtxErr atomic.Value
	for i := range 3 {
		l.addStop(func(ctx context.Context) error {
			ran.Add(1)
			if i == 2 { // runs first: reverse order
				<-ctx.Done()
				return ctx.Err()
			}
			secondCtxErr.Store(ctx.Err() != nil)
			return nil
		})
	}
	start := time.Now()
	err := l.shutdown(t.Context())
	if took := time.Since(start); took > 400*time.Millisecond {
		t.Errorf("shutdown took %v with three stops on an 80ms shared budget", took)
	}
	if ran.Load() != 3 {
		t.Errorf("%d stops ran, want every one run even after the budget was spent", ran.Load())
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("shutdown = %v, want the spent budget reported", err)
	}
}

// A caller that sized its deadline as (unwind + tail) gets the workers the unwind and the whole of
// Shutdown, stops included, ends by that deadline: the tail is carved off the end, not added after.
func TestShutdownTailIsCarvedFromTheCallersDeadline(t *testing.T) {
	const unwind = 200 * time.Millisecond
	newLifecycle := func(t *testing.T) *generationLifecycle {
		l := newGenerationLifecycle(t.Context())
		l.stopBudget = 50 * time.Millisecond
		l.stopGrace = 10 * time.Millisecond
		return l
	}
	deadline := func(t *testing.T, l *generationLifecycle) context.Context {
		ctx, cancel := context.WithTimeout(t.Context(), unwind+l.tail())
		t.Cleanup(cancel)
		return ctx
	}

	// Workers that finish inside the unwind are not reported, though the caller's deadline is
	// later than their finish by less than the tail.
	l := newLifecycle(t)
	l.goRun(func(ctx context.Context) { <-ctx.Done(); time.Sleep(150 * time.Millisecond) })
	probe := addStopProbe(l)
	if err := l.shutdown(deadline(t, l)); err != nil {
		t.Fatalf("shutdown = %v, want a worker inside the unwind not reported", err)
	}
	waitStop(t, probe)

	// A stuck worker is given the unwind, the late wait, then the stops: all by the deadline.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	stuck := newLifecycle(t)
	stuck.goRun(func(context.Context) { <-release })
	stuck.addStop(func(context.Context) error { return nil })
	ctx := deadline(t, stuck)
	end, _ := ctx.Deadline()
	if err := stuck.shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v, want the stuck worker reported", err)
	}
	if late := time.Since(end); late > 100*time.Millisecond {
		t.Errorf("shutdown returned %v after the caller's deadline, want it inside it", late)
	}
}

// A stop that ignores its context cannot hold shutdown forever: it is waited for one grace past the
// budget, then named. A stop that merely honours the deadline is waited for to the end.
func TestShutdownNamesAStopThatIgnoresItsBudget(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 30 * time.Millisecond
	l.stopGrace = 30 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	l.addStop(func(context.Context) error { <-release; return nil })
	start := time.Now()
	err := l.shutdown(t.Context())
	if err == nil || !strings.Contains(err.Error(), "stop still running") {
		t.Fatalf("shutdown = %v, want the unresponsive stop named", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("shutdown took %v, want it bounded by budget plus grace", took)
	}

	// Honouring the deadline slowly (cleanup after ctx.Done) is still finished before the return.
	slow := newGenerationLifecycle(t.Context())
	slow.stopBudget = 30 * time.Millisecond
	slow.stopGrace = 500 * time.Millisecond
	var finished atomic.Bool
	slow.addStop(func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(60 * time.Millisecond)
		finished.Store(true)
		return ctx.Err()
	})
	_ = slow.shutdown(t.Context())
	if !finished.Load() {
		t.Error("a stop that honoured its deadline was still unwinding when shutdown returned")
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
