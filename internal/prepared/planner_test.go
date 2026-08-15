package prepared

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/media"
)

type fixedCandidates struct {
	items []Candidate
	err   error
}

func (f fixedCandidates) Candidates(context.Context, time.Time, time.Time) ([]Candidate, error) {
	return f.items, f.err
}

type recordingPreparation struct {
	mu       sync.Mutex
	requests []Request
	run      func(context.Context, Request) error
}

func (p *recordingPreparation) Prepare(ctx context.Context, request Request) (Publication, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	if p.run != nil {
		return Publication{}, p.run(ctx, request)
	}
	return Publication{}, nil
}

func TestPlannerPreparesUniqueCandidatesInNeedOrder(t *testing.T) {
	now := time.Unix(1_000, 0)
	a := Request{Source: Source{Path: "/a", AudioTrack: 0}, Rendition: baselineRendition()}
	b := Request{Source: Source{Path: "/b", AudioTrack: 1}, Rendition: baselineRendition()}
	work := &recordingPreparation{}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{
			{NeededAt: now.Add(time.Hour), Request: b},
			{NeededAt: now.Add(2 * time.Hour), Request: a},
			{NeededAt: now, Request: a},
		}}, Preparation: work, Pool: media.NewEncodePool(func() int { return 2 }),
		Now: func() time.Time { return now },
	})

	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(work.requests) != 2 || work.requests[0] != a || work.requests[1] != b {
		t.Fatalf("requests = %#v, want [a b] in earliest-need order", work.requests)
	}
}

func TestPlannerYieldsWhenLiveOwnsTheSpareCapacity(t *testing.T) {
	pool := media.NewEncodePool(func() int { return 2 })
	release, ok := pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("foreground setup lease refused")
	}
	defer release()
	work := &recordingPreparation{}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{
			{Request: Request{Source: Source{Path: "/a"}, Rendition: baselineRendition()}},
		}},
		Preparation: work, Pool: pool, Now: time.Now,
	})

	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(work.requests) != 0 {
		t.Fatal("planner started work while live playout owned the spare slot")
	}
}

func TestPlannerTreatsForegroundPreemptionAsAYield(t *testing.T) {
	pool := media.NewEncodePool(func() int { return 2 })
	started := make(chan struct{})
	work := &recordingPreparation{run: func(ctx context.Context, _ Request) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{
			{Request: Request{Source: Source{Path: "/a"}, Rendition: baselineRendition()}},
		}},
		Preparation: work, Pool: pool, Now: time.Now,
	})

	done := make(chan error, 1)
	go func() { done <- p.Run(t.Context()) }()
	<-started
	firstForegroundRelease, ok := pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("first foreground lease refused")
	}
	foregroundRelease, ok := pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("foreground could not preempt preparation")
	}
	firstForegroundRelease()
	foregroundRelease()
	if err := <-done; err != nil {
		t.Fatalf("preempted planner returned an operator-visible failure: %v", err)
	}
}

func TestPlannerContinuesPastOneBadSource(t *testing.T) {
	now := time.Unix(1_000, 0)
	bad := Request{Source: Source{Path: "/bad"}, Rendition: baselineRendition()}
	good := Request{Source: Source{Path: "/good"}, Rendition: baselineRendition()}
	work := &recordingPreparation{run: func(_ context.Context, request Request) error {
		if request == bad {
			return errors.New("broken source")
		}
		return nil
	}}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{
			{NeededAt: now, Request: bad}, {NeededAt: now.Add(time.Minute), Request: good},
		}}, Preparation: work, Pool: media.NewEncodePool(func() int { return 2 }),
		Now: func() time.Time { return now },
	})

	err := p.Run(t.Context())
	if err == nil || len(work.requests) != 2 {
		t.Fatalf("Run error = %v, requests = %d; want error plus continued progress", err, len(work.requests))
	}
}

type recordingRetainer struct {
	calls  int
	budget int64
	result PruneResult
	err    error
}

func (r *recordingRetainer) Prune(_ context.Context, budget int64) (PruneResult, error) {
	r.calls++
	r.budget = budget
	return r.result, r.err
}

func TestPlannerRunsRetentionAfterYieldingPreparation(t *testing.T) {
	pool := media.NewEncodePool(func() int { return 1 }) // no background slot by contract
	retainer := &recordingRetainer{result: PruneResult{RemainingBytes: 700, BudgetBytes: 512}}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{
			{Request: Request{Source: Source{Path: "/a"}, Rendition: baselineRendition()}},
		}},
		Preparation: &recordingPreparation{}, Pool: pool, Retainer: retainer,
		BudgetBytes: func() int64 { return 512 }, Now: time.Now,
	})

	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if retainer.calls != 1 || retainer.budget != 512 {
		t.Fatalf("retention calls = %d at %d bytes, want one at 512", retainer.calls, retainer.budget)
	}
}
