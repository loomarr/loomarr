package prepared_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media"
	"github.com/loomarr/loomarr/internal/prepared"
)

// scaleResolver stops reporting a candidate once its publication completed, as the real resolver
// does when the publication becomes ready.
type scaleResolver struct {
	candidates []prepared.Candidate
	work       *blockingScalePreparation
}

func (r scaleResolver) Plan(context.Context, time.Time, time.Time) (prepared.ReadinessPlan, error) {
	r.work.mu.Lock()
	defer r.work.mu.Unlock()
	pending := make([]prepared.Candidate, 0, len(r.candidates))
	for _, candidate := range r.candidates {
		if !r.work.completed[candidate.Request.Source.SourceID] {
			pending = append(pending, candidate)
		}
	}
	return prepared.ReadinessPlan{Candidates: pending}, nil
}

type blockingScalePreparation struct {
	mu        sync.Mutex
	completed map[string]bool
	started   []string
	start     chan string
	canceled  chan string
	release   chan struct{}
}

func (p *blockingScalePreparation) Prepare(
	ctx context.Context, request prepared.Request,
) (prepared.Publication, error) {
	id := request.Source.SourceID
	p.mu.Lock()
	p.started = append(p.started, id)
	p.mu.Unlock()
	p.start <- id
	select {
	case <-ctx.Done():
		p.canceled <- id
		return prepared.Publication{}, ctx.Err()
	case <-p.release:
		p.mu.Lock()
		p.completed[id] = true
		p.mu.Unlock()
		return prepared.Publication{}, nil
	}
}

func TestPlannerPublicSeamScalesOneHundredChannelPriorityAndPreemption(t *testing.T) {
	const capacity = 12
	now := time.Unix(1_000, 0)
	rendition := prepared.RenditionContract{
		VideoCodec: "h264", AudioCodec: "aac", Width: 1280, Height: 720,
		FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160,
		SegmentDurationMS: 2000, PackagingVersion: 1,
	}
	candidates := make([]prepared.Candidate, 0, 101)
	for i := range 100 {
		class := prepared.CandidateLookahead
		switch {
		case i < 20:
			class = prepared.CandidateCurrent
		case i < 40:
			class = prepared.CandidateNext
		}
		candidates = append(candidates, prepared.Candidate{
			Class: class, NeededAt: now.Add(time.Duration(i) * time.Minute),
			Request: prepared.Request{
				Source: prepared.Source{
					ItemID: "item-" + fmt.Sprintf("%03d", i), SourceID: fmt.Sprintf("source-%03d", i),
					Revision: "revision-1",
				},
				Rendition: rendition,
			},
		})
	}
	// Reverse the resolver output and add a duplicate that would consume one of the first eleven
	// slots if requests were not deduplicated. Planner ordering—not fixture order—must still admit
	// the earliest eleven unique current publications.
	for left, right := 0, len(candidates)-1; left < right; left, right = left+1, right-1 {
		candidates[left], candidates[right] = candidates[right], candidates[left]
	}
	duplicate := candidates[len(candidates)-1]
	duplicate.Class = prepared.CandidateCurrent
	duplicate.NeededAt = now.Add(-time.Minute)
	candidates = append(candidates, duplicate)

	work := &blockingScalePreparation{
		start: make(chan string, 256), canceled: make(chan string, 256), release: make(chan struct{}),
		completed: make(map[string]bool),
	}
	pool := media.NewEncodePool(func() int { return capacity })
	resolver := scaleResolver{candidates: candidates, work: work}
	planner := prepared.NewPlanner(prepared.PlannerDependencies{
		Resolver: resolver, Preparation: work, Pool: pool,
		Now: func() time.Time { return now },
	})
	if err := planner.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	started := make(map[string]bool, capacity-1)
	for range capacity - 1 {
		select {
		case id := <-work.start:
			started[id] = true
		case <-time.After(time.Second):
			t.Fatal("planner did not fill measured N-1 capacity")
		}
	}
	for i := range capacity - 1 {
		id := fmt.Sprintf("source-%03d", i)
		if !started[id] {
			t.Fatalf("initial admitted set = %v, missing urgent %s", started, id)
		}
	}

	reserveRelease, ok := pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("foreground playback was not admitted after preparation drained")
	}
	secondRelease, ok := pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("a second foreground playback lease did not share foreground capacity")
	}
	canceled := make(map[string]bool, capacity-1)
	for range capacity - 1 {
		select {
		case id := <-work.canceled:
			canceled[id] = true
		case <-time.After(time.Second):
			t.Fatal("foreground did not drain every background preparation")
		}
	}
	for id := range started {
		if !canceled[id] {
			t.Fatalf("started background %s was not cancelled for foreground playback", id)
		}
	}
	reserveRelease()
	secondRelease()

	// Playback yielding is not the end of the work: on the next scheduler tick the cancelled wave is
	// requeued at the front and the same most-urgent set is re-admitted, refilling the pool to N-1.
	if err := planner.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := planner.Run(t.Context()); err != nil {
		t.Fatalf("foreground preemption became an operator-visible planner failure: %v", err)
	}
	restarted := make(map[string]bool, capacity-1)
	for range capacity - 1 {
		select {
		case id := <-work.start:
			restarted[id] = true
		case <-time.After(5 * time.Second):
			t.Fatal("planner did not refill the pool after playback released it")
		}
	}
	for id := range started {
		if !restarted[id] {
			t.Fatalf("requeued urgent %s was not re-admitted first: %v", id, restarted)
		}
	}
	close(work.release)
	for range 100 {
		if err := planner.Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := planner.Run(t.Context()); err != nil {
			t.Fatalf("planner failure while draining the schedule: %v", err)
		}
		work.mu.Lock()
		finished := len(work.completed)
		work.mu.Unlock()
		if finished == 100 {
			break
		}
	}
	if err := planner.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	work.mu.Lock()
	startedCount := len(work.started)
	work.mu.Unlock()
	if want := (capacity - 1) + 100; startedCount != want {
		t.Fatalf("planner started %d preparations, want the preempted wave plus every unique candidate = %d", startedCount, want)
	}
}
