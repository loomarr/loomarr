package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The tail a lifecycle keeps for itself is the exported constant callers size their deadlines to.
func TestLifecycleTailIsShutdownTail(t *testing.T) {
	if got := newGenerationLifecycle(t.Context()).tail(); got != ShutdownTail {
		t.Errorf("tail() = %v, want ShutdownTail %v", got, ShutdownTail)
	}
}

// A quiescer is a network-facing resource, so a stop must not begin beside one any more than beside a
// worker: the late wait that workers get covers quiescers too, and the shutdown outcome names a
// quiescer that outlived the deadline.
func TestShutdownWaitsForAQuiescerThatIgnoresItsContext(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 500 * time.Millisecond
	l.stopGrace = 50 * time.Millisecond
	var quiescerRunning, overlap atomic.Bool
	l.addQuiesce(func(context.Context) error {
		quiescerRunning.Store(true)
		time.Sleep(300 * time.Millisecond) // ignores its context
		quiescerRunning.Store(false)
		return nil
	})
	var stopStarted atomic.Bool
	l.addStop(func(context.Context) error {
		stopStarted.Store(true)
		if quiescerRunning.Load() {
			overlap.Store(true)
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	err := l.shutdown(ctx)
	if !stopStarted.Load() {
		t.Fatal("the stop never ran")
	}
	if overlap.Load() {
		t.Error("a stop started while a quiescer was still running")
	}
	if err == nil || !strings.Contains(err.Error(), "quiesce unfinished") {
		t.Errorf("shutdown = %v, want the quiescer that outlived the deadline reported", err)
	}
}

// The late wait is one budget shared by workers and quiescers, not one each, so the documented
// ShutdownTail (late wait, then the stops and their grace) holds with both stuck.
func TestShutdownLateWaitIsOneBudgetForWorkersAndQuiescers(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 150 * time.Millisecond
	l.stopGrace = 20 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	l.addQuiesce(func(context.Context) error { <-release; return nil })
	l.goRun(func(context.Context) { <-release })
	l.addStop(func(context.Context) error { return nil })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := l.shutdown(ctx)
	// 30ms to the deadline, one 150ms late wait, then a prompt stop. Two late waits would be 330ms.
	if took := time.Since(start); took > 280*time.Millisecond {
		t.Errorf("shutdown took %v with a stuck worker and a stuck quiescer, want the late wait shared (about 180ms)", took)
	}
	if err == nil || !strings.Contains(err.Error(), "quiesce unfinished") || !strings.Contains(err.Error(), "workers still running") {
		t.Errorf("shutdown = %v, want both the quiescer and the worker named", err)
	}
}

// The stop phase has one hard stop, the budget plus one grace, however many stops ignore their
// context: each stuck stop does not cost another grace.
func TestShutdownStopPhaseHasOneHardStop(t *testing.T) {
	const stops = 5
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 100 * time.Millisecond
	l.stopGrace = 20 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var ran atomic.Int64
	for range stops {
		l.addStop(func(context.Context) error { ran.Add(1); <-release; return nil })
	}
	start := time.Now()
	err := l.shutdown(t.Context())
	// budget + grace is 120ms. Five graces after the budget would be 200ms.
	if took := time.Since(start); took > 170*time.Millisecond {
		t.Errorf("shutdown took %v with %d stuck stops, want it to end at budget + grace (120ms)", took, stops)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("shutdown = %v, want the stuck stops reported", err)
	}
	// Every stop is started even after the hard stop passed, so a later resource is still released.
	deadline := time.Now().Add(time.Second)
	for ran.Load() != stops && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ran.Load() != stops {
		t.Errorf("%d of %d stops started", ran.Load(), stops)
	}
}

// ShutdownTail is the documented bound: Shutdown outlasts the join by at most late wait plus stop
// budget plus grace, with a worker, a quiescer and every stop stuck at once.
func TestShutdownNeverOutlastsItsTail(t *testing.T) {
	l := newGenerationLifecycle(t.Context())
	l.stopBudget = 100 * time.Millisecond
	l.stopGrace = 20 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	l.addQuiesce(func(context.Context) error { <-release; return nil })
	l.goRun(func(context.Context) { <-release })
	for range 4 {
		l.addStop(func(context.Context) error { <-release; return nil })
	}
	const join = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), join+l.tail())
	defer cancel()
	start := time.Now()
	_ = l.shutdown(ctx)
	if took, bound := time.Since(start), join+l.tail(); took > bound+30*time.Millisecond {
		t.Errorf("shutdown took %v, want it inside the %v bound its caller sized for", took, bound)
	}
}
