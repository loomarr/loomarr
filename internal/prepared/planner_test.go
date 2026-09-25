package prepared

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media"
)

func testSource(id string, audioTrack ...int) Source {
	track := 0
	if len(audioTrack) > 0 {
		track = audioTrack[0]
	}
	return Source{ItemID: "item-" + id, SourceID: "source-" + id, Revision: "revision-" + id, AudioTrack: track}
}

type fixedCandidates struct {
	items     []Candidate
	protected []Specification
	summary   ReadinessSummary
	err       error
}

type countingCandidates struct {
	calls atomic.Int64
	items []Candidate
}

type observingCandidates struct {
	before     ReadinessPlan
	after      ReadinessPlan
	observeErr error
	observeCtx error
	calls      atomic.Int64
}

func (f *observingCandidates) Plan(context.Context, time.Time, time.Time) (ReadinessPlan, error) {
	f.calls.Add(1)
	return f.before, nil
}

func (f *observingCandidates) Observe(ctx context.Context, _ time.Time, _ time.Time) (ReadinessPlan, error) {
	f.calls.Add(1)
	f.observeCtx = ctx.Err()
	return f.after, f.observeErr
}

func (f *countingCandidates) Plan(context.Context, time.Time, time.Time) (ReadinessPlan, error) {
	f.calls.Add(1)
	return ReadinessPlan{Candidates: f.items}, nil
}

func (f fixedCandidates) Plan(context.Context, time.Time, time.Time) (ReadinessPlan, error) {
	return ReadinessPlan{Candidates: f.items, Protected: f.protected, Summary: f.summary}, f.err
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

// pendingCandidates is a resolver whose plan shrinks as publications complete, the way the real
// resolver stops reporting a candidate once its publication is ready. Workers outlive Run, so a
// fixed plan would be re-admitted on every pass.
type pendingCandidates struct {
	mu      sync.Mutex
	items   []Candidate
	summary ReadinessSummary
}

func (r *pendingCandidates) Plan(context.Context, time.Time, time.Time) (ReadinessPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return ReadinessPlan{Candidates: append([]Candidate(nil), r.items...), Summary: r.summary}, nil
}

// complete removes every candidate for request, as a successful publication does.
func (r *pendingCandidates) complete(request Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.items[:0]
	for _, candidate := range r.items {
		if candidate.Request != request {
			kept = append(kept, candidate)
		}
	}
	r.items = kept
}

func (r *pendingCandidates) remaining() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// runUntilReady models scheduler ticks: run a pass, let its workers finish, repeat until the plan
// is empty. It fails the test if that does not converge.
func runUntilReady(t *testing.T, p *Planner, resolver *pendingCandidates) {
	t.Helper()
	// Start quiet: a worker finishing between a pass's plan snapshot and its launch is re-submitted
	// (Prepare then peeks the library and returns at once), which is harmless but would skew counts.
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range 200 {
		if err := p.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := p.Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		if resolver.remaining() == 0 {
			return
		}
	}
	t.Fatalf("planner did not converge: %d candidates remain", resolver.remaining())
}

func TestPlannerPreparesUniqueCandidatesInNeedOrder(t *testing.T) {
	now := time.Unix(1_000, 0)
	a := Request{Source: testSource("a"), Rendition: baselineRendition()}
	b := Request{Source: testSource("b", 1), Rendition: baselineRendition()}
	resolver := &pendingCandidates{items: []Candidate{
		{NeededAt: now.Add(time.Hour), Request: b},
		{NeededAt: now.Add(2 * time.Hour), Request: a},
		{NeededAt: now, Request: a},
	}}
	work := &recordingPreparation{run: func(_ context.Context, request Request) error {
		resolver.complete(request)
		return nil
	}}
	// Capacity two leaves one background slot, so the ticks are what serialise a then b.
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: work, Pool: media.NewEncodePool(func() int { return 2 }),
		Now: func() time.Time { return now },
	})

	runUntilReady(t, p, resolver)
	if len(work.requests) != 2 || work.requests[0] != a || work.requests[1] != b {
		t.Fatalf("requests = %#v, want [a b] in earliest-need order", work.requests)
	}
}

func TestPlannerFillsAndRefillsMeasuredBackgroundCapacity(t *testing.T) {
	const (
		capacity   = 12
		candidates = 100
	)
	now := time.Unix(1_000, 0)
	items := make([]Candidate, candidates)
	for i := range items {
		items[i] = Candidate{
			NeededAt: now.Add(time.Duration(i) * time.Second),
			Request:  Request{Source: testSource(string(rune(i + 1))), Rendition: baselineRendition()},
		}
	}
	resolver := &pendingCandidates{items: items}
	started := make(chan struct{}, capacity-1+len(items)) // room for the requeued wave and refills
	releaseFirstWave := make(chan struct{})
	var calls atomic.Int64
	var active atomic.Int64
	var peak atomic.Int64
	work := &recordingPreparation{run: func(_ context.Context, request Request) error {
		defer resolver.complete(request)
		call := calls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for {
			prior := peak.Load()
			if prior >= current || peak.CompareAndSwap(prior, current) {
				break
			}
		}
		if call <= capacity-1 {
			started <- struct{}{}
			<-releaseFirstWave
		}
		return nil
	}}
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: work,
		Pool: media.NewEncodePool(func() int { return capacity }), Now: func() time.Time { return now },
	})
	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range capacity - 1 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("planner did not fill measured background capacity")
		}
	}
	if got := peak.Load(); got != capacity-1 {
		t.Fatalf("first-wave concurrency = %d, want %d", got, capacity-1)
	}
	close(releaseFirstWave)
	// Refill of the freed slots happens on the following scheduler ticks.
	runUntilReady(t, p, resolver)
	if got := calls.Load(); got != candidates {
		t.Fatalf("preparation calls = %d, want %d after refill", got, candidates)
	}
	if got := peak.Load(); got != capacity-1 {
		t.Fatalf("peak preparation concurrency = %d, want %d", got, capacity-1)
	}
}

// blockingPlanCandidates holds the first Plan call open so a second Run can overlap the pass.
type blockingPlanCandidates struct {
	countingCandidates
	entered chan struct{}
	release chan struct{}
}

func (f *blockingPlanCandidates) Plan(ctx context.Context, from, to time.Time) (ReadinessPlan, error) {
	plan, err := f.countingCandidates.Plan(ctx, from, to)
	if f.calls.Load() == 1 {
		close(f.entered)
		<-f.release
	}
	return plan, err
}

func TestPlannerCoalescesOverlappingRuns(t *testing.T) {
	resolver := &blockingPlanCandidates{
		countingCandidates: countingCandidates{items: []Candidate{{
			Request: Request{Source: testSource("warming"), Rendition: baselineRendition()},
		}}},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: &recordingPreparation{},
		Pool: media.NewEncodePool(func() int { return 2 }), Now: time.Now,
	})
	first := make(chan error, 1)
	go func() { first <- p.Run(t.Context()) }()
	<-resolver.entered
	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("resolver calls during overlap = %d, want one coalesced pass", got)
	}
	if got := p.Status(); !got.Running {
		t.Fatalf("overlapping yield cleared in-progress status: %+v", got)
	}
	close(resolver.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

// A later pass must not start a publication a previous pass's worker is still producing.
func TestPlannerDoesNotRestartAPublicationAlreadyInFlight(t *testing.T) {
	started := make(chan struct{}, 4)
	finish := make(chan struct{})
	resolver := &countingCandidates{items: []Candidate{{
		Request: Request{Source: testSource("long"), Rendition: baselineRendition()},
	}}}
	work := &recordingPreparation{run: func(context.Context, Request) error {
		started <- struct{}{}
		<-finish
		return nil
	}}
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: work,
		Pool: media.NewEncodePool(func() int { return 4 }), Now: time.Now,
	})
	for range 3 {
		if err := p.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	<-started
	close(finish)
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(work.requests) != 1 {
		t.Fatalf("publication started %d times across three passes, want once", len(work.requests))
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
			{Request: Request{Source: testSource("a"), Rendition: baselineRendition()}},
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
	var once sync.Once
	work := &recordingPreparation{run: func(ctx context.Context, _ Request) error {
		first := false
		once.Do(func() { first = true; close(started) })
		if !first {
			return nil // the requeued attempt completes once playback has yielded
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{
			{Request: Request{Source: testSource("a"), Rendition: baselineRendition()}},
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
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The next tick re-admits the preempted publication; the preemption itself was never a failure.
	if err := p.Run(t.Context()); err != nil {
		t.Fatalf("pass after preemption reported the preemption as a failure: %v", err)
	}
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(work.requests) != 2 {
		t.Fatalf("preparation attempts = %d, want the preempted attempt plus its requeue", len(work.requests))
	}
}

func TestPlannerForegroundPreemptionRequeuesAndRefillsAcrossMeasuredCapacity(t *testing.T) {
	const capacity = 4
	now := time.Unix(1_000, 0)
	items := make([]Candidate, 10)
	for i := range items {
		items[i] = Candidate{
			NeededAt: now.Add(time.Duration(i) * time.Minute),
			Request:  Request{Source: testSource(fmt.Sprintf("%02d", i)), Rendition: baselineRendition()},
		}
	}
	resolver := &pendingCandidates{items: items}
	started := make(chan struct{}, capacity-1+len(items)) // room for the requeued wave and refills
	var calls atomic.Int64
	work := &recordingPreparation{run: func(ctx context.Context, request Request) error {
		if calls.Add(1) > capacity-1 {
			resolver.complete(request) // the requeued wave and refills finish at once
			return nil
		}
		started <- struct{}{}
		<-ctx.Done() // the first wave holds its slots until live playback preempts it
		return ctx.Err()
	}}
	pool := media.NewEncodePool(func() int { return capacity })
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: work, Pool: pool,
		Now: func() time.Time { return now },
	})
	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range capacity - 1 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("planner did not fill the background pool")
		}
	}
	reserveRelease, ok := pool.AcquireForeground(t.Context())
	if !ok {
		t.Fatal("live request could not take the reserved slot")
	}
	second := make(chan func(), 1)
	go func() {
		release, admitted := pool.AcquireForeground(t.Context())
		if !admitted {
			second <- nil
			return
		}
		second <- release
	}()
	var preemptedRelease func()
	select {
	case preemptedRelease = <-second:
		if preemptedRelease == nil {
			t.Fatal("live request was not admitted after preparation preemption")
		}
	case <-time.After(time.Second):
		t.Fatal("live request did not promptly preempt preparation")
	}
	reserveRelease()
	preemptedRelease()
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The three preempted publications are requeued ahead of everything else (they are the most
	// urgent) on the following ticks and every candidate then completes: 3 cancelled attempts +
	// all 10 candidates. Runs report no failure for the preemption.
	runUntilReady(t, p, resolver)
	if got := calls.Load(); got != int64(capacity-1+len(items)) {
		t.Fatalf("preparation calls after live preemption = %d, want %d: the preempted wave requeued and refilled", got, capacity-1+len(items))
	}
}

func TestPlannerContinuesPastOneBadSource(t *testing.T) {
	now := time.Unix(1_000, 0)
	bad := Request{Source: testSource("bad"), Rendition: baselineRendition()}
	good := Request{Source: testSource("good"), Rendition: baselineRendition()}
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

	// Workers outlive the pass, so a source's failure is reported by the pass that follows it.
	// Capacity two leaves one slot: bad runs first, fails, and only then does good get its turn.
	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	err := p.Run(t.Context())
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err == nil || !strings.Contains(err.Error(), "broken source") {
		t.Fatalf("Run error = %v, want the earlier publication failure", err)
	}
	if !slices.Contains(work.requests, good) {
		t.Fatalf("requests = %v: one bad source starved the good one", work.requests)
	}
}

type recordingRetainer struct {
	calls     int
	budget    int64
	protected []Specification
	ctxErr    error
	result    PruneResult
	err       error
}

func (r *recordingRetainer) Prune(
	ctx context.Context, budget int64, protected []Specification,
) (PruneResult, error) {
	r.calls++
	r.budget = budget
	r.protected = append([]Specification(nil), protected...)
	r.ctxErr = ctx.Err()
	return r.result, r.err
}

func TestPlannerRunsRetentionAfterYieldingPreparation(t *testing.T) {
	pool := media.NewEncodePool(func() int { return 1 }) // no background slot by contract
	retainer := &recordingRetainer{result: PruneResult{RemainingBytes: 700, BudgetBytes: 512}}
	protected := Specification{SourceFingerprint: "scheduled", Rendition: baselineRendition()}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{protected: []Specification{protected}, items: []Candidate{
			{Request: Request{Source: testSource("a"), Rendition: baselineRendition()}},
		}},
		Preparation: &recordingPreparation{}, Pool: pool, Retainer: retainer,
		BudgetBytes: func() int64 { return 512 }, Now: time.Now,
	})

	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if retainer.calls != 1 || retainer.budget != 512 || len(retainer.protected) != 1 || retainer.protected[0] != protected {
		t.Fatalf("retention calls = %d at %d bytes, want one at 512", retainer.calls, retainer.budget)
	}
}

func TestPlannerStatusReportsResolvedReadinessAndRetentionAfterYield(t *testing.T) {
	now := time.Unix(20_000, 0)
	retainer := &recordingRetainer{result: PruneResult{
		RemainingBytes: 700, BudgetBytes: 512, ProtectedBytes: 400,
		PublicationsEvicted: 2, BytesEvicted: 100,
	}}
	wantReadiness := ReadinessSummary{
		Channels: 100, ReadyChannels: 84,
		ScheduledBindings: 300, ReadyBindings: 260, MissingBindings: 40,
		QueuedPublications: 16,
	}
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{summary: wantReadiness, items: []Candidate{{
			Request: Request{Source: testSource("warming"), Rendition: baselineRendition()},
		}}},
		Preparation: &recordingPreparation{},
		Pool:        media.NewEncodePool(func() int { return 1 }), // no background slot: a normal yield
		Retainer:    retainer,
		BudgetBytes: func() int64 { return 512 },
		Now:         func() time.Time { return now },
	})

	if got := p.Status(); !got.Available || got.Running || !got.LastRunAt.IsZero() {
		t.Fatalf("initial status = %+v, want available but not yet run", got)
	}
	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := PlannerStatus{
		Available: true, LastRunAt: now, Readiness: wantReadiness,
		Retention: RetentionStatus{
			RemainingBytes: 700, BudgetBytes: 512, ProtectedBytes: 400,
			PublicationsEvicted: 2, BytesEvicted: 100,
		},
	}
	if got := p.Status(); got != want {
		t.Fatalf("status = %+v, want %+v", got, want)
	}
}

func TestPlannerStatusObservesResultingReadinessAfterWork(t *testing.T) {
	now := time.Unix(21_000, 0)
	request := Request{Source: testSource("warming"), Rendition: baselineRendition()}
	before := ReadinessPlan{
		Candidates: []Candidate{{Request: request}},
		Summary: ReadinessSummary{
			Channels: 100, ScheduledBindings: 100, MissingBindings: 100, QueuedPublications: 100,
		},
	}
	after := ReadinessPlan{Summary: ReadinessSummary{
		Channels: 100, ReadyChannels: 100, ScheduledBindings: 100, ReadyBindings: 100,
	}}
	resolver := &observingCandidates{before: before, after: after}
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: &recordingPreparation{},
		Pool: media.NewEncodePool(func() int { return 2 }), Now: func() time.Time { return now },
	})

	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := p.Status().Readiness; got != after.Summary {
		t.Fatalf("post-work readiness = %+v, want resulting %+v", got, after.Summary)
	}
	if got := resolver.calls.Load(); got != 2 {
		t.Fatalf("resolver calls = %d, want plan plus lookup-only observation", got)
	}
}

type expiringPlannerContext struct {
	context.Context
	deadline time.Time
	done     chan struct{}
	expired  atomic.Bool
	once     sync.Once
}

func newExpiringPlannerContext(parent context.Context) *expiringPlannerContext {
	return &expiringPlannerContext{
		Context: parent, deadline: time.Now().Add(time.Hour),
		done: make(chan struct{}),
	}
}

func (c *expiringPlannerContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c *expiringPlannerContext) Done() <-chan struct{}       { return c.done }
func (c *expiringPlannerContext) Err() error {
	if c.expired.Load() {
		return context.DeadlineExceeded
	}
	return nil
}
func (c *expiringPlannerContext) expire() {
	c.once.Do(func() {
		c.expired.Store(true)
		close(c.done)
	})
}

// expiringOnPlan lets the scheduler's job deadline hit mid-pass, after the schedule resolved.
type expiringOnPlan struct {
	*observingCandidates
	ctx *expiringPlannerContext
}

func (e expiringOnPlan) Plan(ctx context.Context, from, to time.Time) (ReadinessPlan, error) {
	plan, err := e.observingCandidates.Plan(ctx, from, to)
	e.ctx.expire()
	return plan, err
}

// Publication workers no longer share the pass's deadline, so the deadline can only interrupt the
// pass itself; it must still observe and retain under a live finalization context, and must not
// admit new work with a dead context.
func TestPlannerDeadlineDuringPassStillObservesAndRetains(t *testing.T) {
	now := time.Unix(21_625, 0)
	request := Request{Source: testSource("deadline"), Rendition: baselineRendition()}
	protected := Specification{SourceFingerprint: "current", Rendition: baselineRendition()}
	resolver := &observingCandidates{
		before: ReadinessPlan{Candidates: []Candidate{{Request: request}}},
		after:  ReadinessPlan{Protected: []Specification{protected}},
	}
	retainer := &recordingRetainer{}
	ctx := newExpiringPlannerContext(t.Context())
	work := &recordingPreparation{}
	p := NewPlanner(PlannerDependencies{
		Resolver: expiringOnPlan{observingCandidates: resolver, ctx: ctx}, Preparation: work,
		Pool: media.NewEncodePool(func() int { return 2 }), Retainer: retainer,
		BudgetBytes: func() int64 { return 512 }, Now: func() time.Time { return now },
	})

	err := p.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want deadline exceeded after finalization", err)
	}
	if len(work.requests) != 0 {
		t.Fatalf("planner admitted %d publications after its deadline", len(work.requests))
	}
	if got := resolver.calls.Load(); got != 2 || resolver.observeCtx != nil {
		t.Fatalf("post-deadline observation = calls %d ctx %v, want one live finalization call", got, resolver.observeCtx)
	}
	if retainer.calls != 1 || retainer.ctxErr != nil || len(retainer.protected) != 1 || retainer.protected[0] != protected {
		t.Fatalf("post-deadline retention = calls %d ctx %v protected %+v", retainer.calls, retainer.ctxErr, retainer.protected)
	}
}

func TestPlannerObservationFailurePreservesTheResolvedHotSet(t *testing.T) {
	now := time.Unix(21_750, 0)
	protected := Specification{SourceFingerprint: "current", Rendition: baselineRendition()}
	wantSummary := ReadinessSummary{Channels: 100, ReadyChannels: 99, MissingBindings: 1}
	resolver := &observingCandidates{
		before:     ReadinessPlan{Protected: []Specification{protected}, Summary: wantSummary},
		after:      ReadinessPlan{},
		observeErr: errors.New("transient schedule read"),
	}
	retainer := &recordingRetainer{}
	p := NewPlanner(PlannerDependencies{
		Resolver: resolver, Preparation: &recordingPreparation{},
		Pool: media.NewEncodePool(func() int { return 2 }), Retainer: retainer,
		BudgetBytes: func() int64 { return 512 }, Now: func() time.Time { return now },
	})

	err := p.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "transient schedule read") {
		t.Fatalf("Run error = %v, want observation failure", err)
	}
	if retainer.calls != 1 || len(retainer.protected) != 1 || retainer.protected[0] != protected {
		t.Fatalf("retention lost pre-work hot set after observation failure: %+v", retainer.protected)
	}
	if got := p.Status().Readiness; got != wantSummary {
		t.Fatalf("status after failed observation = %+v, want last complete plan %+v", got, wantSummary)
	}
}

func TestPlannerStatusReportsPassInProgress(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	p := NewPlanner(PlannerDependencies{
		Resolver: fixedCandidates{items: []Candidate{{
			Request: Request{Source: testSource("warming"), Rendition: baselineRendition()},
		}}},
		Preparation: &recordingPreparation{run: func(context.Context, Request) error {
			close(started)
			<-finish
			return nil
		}},
		Pool: media.NewEncodePool(func() int { return 2 }), Now: time.Now,
	})
	if err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-started
	// The pass has returned but its publication is still encoding: status must say so.
	if got := p.Status(); !got.Running || got.LastRunAt.IsZero() {
		t.Fatalf("in-progress status = %+v, want running with the pass recorded", got)
	}
	close(finish)
	if err := p.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := p.Status(); got.Running || got.LastRunAt.IsZero() {
		t.Fatalf("completed status = %+v", got)
	}
}

func TestPlannerStatusPreservesUnavailableReason(t *testing.T) {
	p := NewPlanner(PlannerDependencies{UnavailableReason: "prepared volume is read-only"})
	if got := p.Status(); got.Available || got.UnavailableReason != "prepared volume is read-only" {
		t.Fatalf("status = %+v", got)
	}
}
