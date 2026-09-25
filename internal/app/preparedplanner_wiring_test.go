package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
)

// wiringResolver is the planner's resolver at the production seam: it reports a fixed schedule
// summary the way preparedRuntimeResolver does, so status assertions can distinguish "no pass
// finished" from "nothing scheduled".
type wiringResolver struct{ plan prepared.ReadinessPlan }

func (r wiringResolver) Plan(context.Context, time.Time, time.Time) (prepared.ReadinessPlan, error) {
	return r.plan, nil
}

// wiringPreparation blocks every publication until released or cancelled, counting how many are
// in flight at once.
type wiringPreparation struct {
	inFlight atomic.Int32
	peak     atomic.Int32
	started  chan struct{}
	release  chan struct{}
	drain    time.Duration // time a cancelled publication keeps its lease tearing down
}

func (p *wiringPreparation) Prepare(ctx context.Context, _ prepared.Request) (prepared.Publication, error) {
	now := p.inFlight.Add(1)
	defer p.inFlight.Add(-1)
	for {
		peak := p.peak.Load()
		if now <= peak || p.peak.CompareAndSwap(peak, now) {
			break
		}
	}
	p.started <- struct{}{}
	select {
	case <-ctx.Done():
		time.Sleep(p.drain)
		return prepared.Publication{}, ctx.Err()
	case <-p.release:
		return prepared.Publication{}, nil
	}
}

// productionShapedPlanner wires the planner to the pool exactly as buildplayout does: the dynamic
// pool from newPreparedEncodePool fed by measured capacity twelve, an operator cap of four.
func productionShapedPlanner(t *testing.T, needed int) (*prepared.Planner, *wiringPreparation, *waitablePool) {
	t.Helper()
	pool := newPreparedEncodePool(
		func() playout.Encoder { return playout.EncoderNVENC },
		func() int { return 12 },
		func(measured int) int { return playout.EffectiveCapacity(measured, 4, 0) },
	)
	now := time.Unix(1_000, 0)
	candidates := make([]prepared.Candidate, 0, needed)
	for i := range needed {
		candidates = append(candidates, prepared.Candidate{
			Class: prepared.CandidateCurrent, NeededAt: now.Add(time.Duration(i) * time.Minute),
			Request: prepared.Request{Source: prepared.Source{
				ItemID: fmt.Sprintf("item-%d", i), SourceID: fmt.Sprintf("source-%d", i), Revision: "r1",
			}},
		})
	}
	preparation := &wiringPreparation{
		started: make(chan struct{}, 64), release: make(chan struct{}),
	}
	planner := prepared.NewPlanner(prepared.PlannerDependencies{
		Resolver: wiringResolver{plan: prepared.ReadinessPlan{
			Candidates: candidates,
			Summary: prepared.ReadinessSummary{
				Channels: 4, ScheduledBindings: needed, MissingBindings: needed,
				QueuedPublications: needed,
			},
		}},
		Preparation: preparation, Pool: pool, Now: func() time.Time { return now },
	})
	return planner, preparation, &waitablePool{pool: pool}
}

type waitablePool struct {
	pool interface {
		AcquireForeground(context.Context) (func(), bool)
	}
}

func waitStarted(t *testing.T, p *wiringPreparation, n int) {
	t.Helper()
	for range n {
		select {
		case <-p.started:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d preparations started", p.inFlight.Load(), n)
		}
	}
}

func TestPreparedPlannerAtCapacityFourRunsThreeConcurrentPublications(t *testing.T) {
	planner, preparation, _ := productionShapedPlanner(t, 8)
	done := make(chan error, 1)
	go func() { done <- planner.Run(t.Context()) }()

	waitStarted(t, preparation, 3)
	select {
	case <-preparation.started:
		t.Fatal("a fourth publication started: the reserved foreground slot was consumed")
	case <-time.After(300 * time.Millisecond):
	}
	if got := preparation.inFlight.Load(); got != 3 {
		t.Fatalf("in-flight publications = %d, want capacity 4 minus the foreground reserve = 3", got)
	}
	close(preparation.release)
	if err := <-done; err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestPreparedPlannerStatusReportsProgressDuringAPass(t *testing.T) {
	planner, preparation, _ := productionShapedPlanner(t, 8)
	done := make(chan error, 1)
	go func() { done <- planner.Run(t.Context()) }()
	defer func() { close(preparation.release); <-done }()

	waitStarted(t, preparation, 3)
	status := planner.Status()
	if !status.Running {
		t.Fatal("status is not running while publications are in flight")
	}
	if status.Readiness.ScheduledBindings != 8 || status.Readiness.QueuedPublications != 8 {
		t.Fatalf("status mid-pass = %+v, want the resolved schedule (8 bindings, 8 queued), not zeros",
			status.Readiness)
	}
}

func TestPreparedPlannerLiveSessionsPreemptBackgroundInsteadOfFallingToSoftware(t *testing.T) {
	planner, preparation, pool := productionShapedPlanner(t, 8)
	// A real preparation keeps its lease while it tears the staging workspace down (tens of
	// thousands of segment files); that must not count against a live session's bounded wait.
	preparation.drain = 900 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- planner.Run(t.Context()) }()
	defer func() { close(preparation.release); <-done }()

	waitStarted(t, preparation, 3)
	admitted := make(chan bool, 2)
	var releases sync.Map
	for i := range 2 {
		go func() {
			release, ok := pool.pool.AcquireForeground(t.Context())
			if ok {
				releases.Store(i, release)
			}
			admitted <- ok
		}()
	}
	for range 2 {
		if !<-admitted {
			t.Fatal("a live session was refused a GPU slot and would fall back to software while " +
				"three background preparations held capacity four")
		}
	}
	releases.Range(func(_, release any) bool { release.(func())(); return true })
}

func TestPreparedPlannerRefillsAfterLiveSessionEndsButNotWhileItHoldsASlot(t *testing.T) {
	planner, preparation, pool := productionShapedPlanner(t, 8)
	done := make(chan error, 1)
	go func() { done <- planner.Run(t.Context()) }()
	defer func() { close(preparation.release); <-done }()

	waitStarted(t, preparation, 3)
	release, ok := pool.pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("live session refused")
	}
	select {
	case <-preparation.started:
		t.Fatal("background re-grabbed a slot while a live session held one")
	case <-time.After(500 * time.Millisecond):
	}
	release()
	waitStarted(t, preparation, 3) // preempted work is requeued and the pool refills to N-1
	if got := preparation.inFlight.Load(); got != 3 {
		t.Fatalf("in-flight after the live session ended = %d, want 3", got)
	}
}
