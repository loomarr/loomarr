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
	p := NewPlanner(fixedCandidates{items: []Candidate{
		{NeededAt: now.Add(time.Hour), Request: b},
		{NeededAt: now.Add(2 * time.Hour), Request: a},
		{NeededAt: now, Request: a},
	}}, work, media.NewEncodePool(func() int { return 2 }), func() time.Time { return now })

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
	p := NewPlanner(fixedCandidates{items: []Candidate{{Request: Request{
		Source: Source{Path: "/a"}, Rendition: baselineRendition(),
	}}}}, work, pool, time.Now)

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
	p := NewPlanner(fixedCandidates{items: []Candidate{{Request: Request{
		Source: Source{Path: "/a"}, Rendition: baselineRendition(),
	}}}}, work, pool, time.Now)

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
	p := NewPlanner(fixedCandidates{items: []Candidate{
		{NeededAt: now, Request: bad}, {NeededAt: now.Add(time.Minute), Request: good},
	}}, work, media.NewEncodePool(func() int { return 2 }), func() time.Time { return now })

	err := p.Run(t.Context())
	if err == nil || len(work.requests) != 2 {
		t.Fatalf("Run error = %v, requests = %d; want error plus continued progress", err, len(work.requests))
	}
}
