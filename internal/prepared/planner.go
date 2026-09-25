package prepared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/media"
)

const preparationLookahead = 6 * time.Hour

// preparationRetryBackoff is how long a failed publication waits before it is admitted again. It is
// internal policy, not an operator tuning surface.
const preparationRetryBackoff = 5 * time.Minute

// preparationFinalizationTimeout bounds the recovery path used when the scheduler's job deadline
// expires mid-pass. It gives lookup-only observation and retention a final cancellation window
// without detaching ordinary shutdown cancellation.
const preparationFinalizationTimeout = 30 * time.Second

// CandidateClass orders the bounded readiness frontier. Zero is current so older callers and test
// fixtures that omit the class remain maximally urgent.
type CandidateClass uint8

const (
	CandidateCurrent CandidateClass = iota
	CandidateNext
	CandidateLookahead
)

// Candidate is one immutable source/rendition needed by the accepted schedule. Class then NeededAt
// control priority; Channel identity is deliberately absent because publications are shared.
type Candidate struct {
	Class    CandidateClass
	NeededAt time.Time
	Request  Request
}

// ReadinessPlan separates missing work from the complete accepted schedule. Protected includes
// every ready publication still referenced by that schedule, even when no encoder slot is free.
type ReadinessPlan struct {
	Candidates []Candidate
	Protected  []Specification
	Summary    ReadinessSummary
}

// ReadinessSummary is the schedule-level result of one resolved lookahead window. Bindings count
// Channel/item pairs rather than publications because one shared publication may make many
// Channels ready.
type ReadinessSummary struct {
	Channels           int
	ReadyChannels      int
	ScheduledBindings  int
	ReadyBindings      int
	MissingBindings    int
	QueuedPublications int
}

// RetentionStatus is the durable store result from the same scheduler pass as readiness.
type RetentionStatus struct {
	RemainingBytes      int64
	BudgetBytes         int64
	ProtectedBytes      int64
	PublicationsEvicted int
	BytesEvicted        int64
	StagingRemoved      int
}

// PlannerStatus is the planner-owned operational snapshot projected by the playout status API.
// A zero LastRunAt means no pass has completed; zero counts must not be interpreted as all ready.
type PlannerStatus struct {
	Available         bool
	UnavailableReason string
	Running           bool
	LastRunAt         time.Time
	LastError         string
	Readiness         ReadinessSummary
	Retention         RetentionStatus
}

// CandidateResolver reads the authoritative schedule and returns stable Inventory sources.
// Implemented at composition, where channels, source access, and audio selection meet.
type CandidateResolver interface {
	Plan(context.Context, time.Time, time.Time) (ReadinessPlan, error)
}

// ReadinessObserver is the lookup-only post-work half implemented by the runtime resolver. Keeping
// it optional preserves narrow test and embedded resolvers while production status reports the
// state resulting from this pass rather than the state before it.
type ReadinessObserver interface {
	Observe(context.Context, time.Time, time.Time) (ReadinessPlan, error)
}

// Preparation publishes one request. Preparer implements it; the interface keeps Planner focused
// on schedule priority rather than fingerprinting or packaging internals.
type Preparation interface {
	Prepare(context.Context, Request) (Publication, error)
}

// Retainer owns the lifecycle of the same immutable store preparation writes.
type Retainer interface {
	Prune(context.Context, int64, []Specification) (PruneResult, error)
}

// PlannerDependencies makes the readiness control plane's ownership explicit. Preparation and
// retention are both started by one scheduler pass so lifecycle cannot drift into a bolt-on task,
// but both run under the Planner's lifecycle, so the pass itself stays short.
type PlannerDependencies struct {
	Resolver          CandidateResolver
	Preparation       Preparation
	Pool              *media.EncodePool
	Retainer          Retainer
	BudgetBytes       func() int64
	Now               func() time.Time
	Log               *slog.Logger
	UnavailableReason string
	// Lifecycle owns every publication worker. It must outlive individual scheduler passes and be
	// cancelled only at shutdown; nil means workers run until Wait (tests, embedded use).
	Lifecycle context.Context
}

// Planner is the readiness control plane. It can publish media but cannot mutate a schedule, and
// every encode runs under the shared preemptible background lease.
type Planner struct {
	resolver CandidateResolver
	preparer Preparation
	pool     *media.EncodePool
	retainer Retainer
	budget   func() int64
	now      func() time.Time
	log      *slog.Logger
	runMu    sync.Mutex
	statusMu sync.RWMutex
	status   PlannerStatus

	lifecycle context.Context
	workerMu  sync.Mutex
	workers   map[Request]context.Context
	workerWG  sync.WaitGroup
	failures  []error
	failedAt  map[Request]time.Time
	retaining bool
}

func NewPlanner(deps PlannerDependencies) *Planner {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Log == nil {
		deps.Log = slog.New(slog.DiscardHandler)
	}
	available := deps.UnavailableReason == "" && deps.Resolver != nil && deps.Preparation != nil && deps.Pool != nil
	reason := deps.UnavailableReason
	if !available && reason == "" {
		reason = "the prepared playout planner is not wired"
	}
	if deps.Lifecycle == nil {
		deps.Lifecycle = context.Background()
	}
	return &Planner{
		lifecycle: deps.Lifecycle, workers: make(map[Request]context.Context),
		failedAt: make(map[Request]time.Time),
		resolver: deps.Resolver, preparer: deps.Preparation, pool: deps.Pool,
		retainer: deps.Retainer, budget: deps.BudgetBytes, now: deps.Now, log: deps.Log,
		status: PlannerStatus{Available: available, UnavailableReason: reason},
	}
}

// Status returns the latest immutable operational snapshot without touching the schedule or disk.
// Running is true while a pass runs and while any publication worker is still in flight: workers
// outlive the pass that started them, and an idle status during a multi-hour encode would mislead.
func (p *Planner) Status() PlannerStatus {
	if p == nil {
		return PlannerStatus{UnavailableReason: "the prepared playout planner is not wired"}
	}
	p.statusMu.RLock()
	status := p.status
	p.statusMu.RUnlock()
	p.workerMu.Lock()
	status.Running = status.Running || len(p.workers) > 0
	p.workerMu.Unlock()
	return status
}

// Run prepares as much of the next schedule window as spare hardware permits. A foreground
// preemption or a busy pool is a normal yield, not a failed task. Independent source failures are
// joined after the pass so one corrupt file cannot starve every later candidate.
func (p *Planner) Run(ctx context.Context) (runErr error) {
	if p == nil {
		return nil
	}
	if !p.runMu.TryLock() {
		return nil
	}
	defer p.runMu.Unlock()
	p.statusMu.Lock()
	p.status.Running = true
	p.statusMu.Unlock()
	var errs []error
	var plan ReadinessPlan
	finalizationCtx := ctx
	defer func() {
		p.statusMu.Lock()
		p.status.Running = false
		p.status.LastRunAt = p.now()
		p.status.Readiness = plan.Summary
		p.status.LastError = ""
		if runErr != nil {
			p.status.LastError = runErr.Error()
		}
		p.statusMu.Unlock()
	}()
	if p.resolver != nil && p.preparer != nil && p.pool != nil {
		now := p.now()
		var resolveErr error
		plan, resolveErr = p.resolver.Plan(ctx, now, now.Add(preparationLookahead))
		errs = append(errs, resolveErr)
		p.publishReadiness(plan.Summary)
		p.launch(ctx, uniqueCandidates(plan.Candidates))
		preparationErrs := p.takeFailures()
		errs = append(errs, preparationErrs...)
		if ctxErr := ctx.Err(); ctxErr != nil {
			if !errors.Is(ctxErr, context.DeadlineExceeded) {
				return ctxErr
			}
			errs = append(errs, ctxErr)
			var cancel context.CancelFunc
			finalizationCtx, cancel = context.WithTimeout(
				context.WithoutCancel(ctx), preparationFinalizationTimeout,
			)
			defer cancel()
		}
		if observer, ok := p.resolver.(ReadinessObserver); ok {
			observed, observeErr := observer.Observe(finalizationCtx, now, now.Add(preparationLookahead))
			if observeErr == nil {
				plan = observed
			}
			errs = append(errs, observeErr)
		}
	}
	if p.retainer != nil && p.budget != nil {
		p.startRetention(p.budget(), plan.Protected)
	}
	runErr = errors.Join(errs...)
	return runErr
}

// startRetention runs one retention sweep under the Planner's lifecycle and returns immediately.
// A sweep stats every file of every publication, so its cost grows with the store rather than with
// the schedule window; run inside the pass it could outlast the pass's short ceiling and be cut
// off every minute. It is single-flight: a sweep still running when the next pass arrives is left
// to finish, and that pass's protected set is dropped (the next one carries a fresh set).
func (p *Planner) startRetention(budget int64, protected []Specification) {
	if p.lifecycle.Err() != nil {
		return
	}
	p.workerMu.Lock()
	if p.retaining {
		p.workerMu.Unlock()
		return
	}
	p.retaining = true
	p.workerMu.Unlock()
	p.workerWG.Add(1)
	go func() {
		defer p.workerWG.Done()
		result, err := p.retainer.Prune(p.lifecycle, budget, protected)
		p.workerMu.Lock()
		p.retaining = false
		p.workerMu.Unlock()
		p.statusMu.Lock()
		p.status.Retention = retentionStatusFrom(result)
		p.statusMu.Unlock()
		if err != nil {
			if p.lifecycle.Err() == nil {
				p.log.Warn("retain prepared media failed", "err", err)
			}
		}
		fields := []any{
			"bytes", result.RemainingBytes, "budget", result.BudgetBytes,
			"protected_bytes", result.ProtectedBytes,
			"publications_evicted", result.PublicationsEvicted,
			"bytes_evicted", result.BytesEvicted, "staging_removed", result.StagingRemoved,
		}
		if result.RemainingBytes > result.BudgetBytes && result.BudgetBytes > 0 {
			p.log.Warn("prepared media remains over its soft budget because recent playback is protected", fields...)
		} else {
			p.log.Info("prepared media retention pass", fields...)
		}
	}()
}

// publishReadiness makes the resolved schedule visible while the pass is still running; the final
// deferred write in Run replaces it with the post-work observation.
func (p *Planner) publishReadiness(summary ReadinessSummary) {
	p.statusMu.Lock()
	p.status.Readiness = summary
	p.statusMu.Unlock()
}

// launch starts a worker for each candidate, most urgent first, until the pool refuses admission,
// and returns without waiting for any of them. Workers belong to the Planner's lifecycle, not to
// this pass: a publication routinely outlasts the scheduler's job timeout (a feature-length
// programme at 1.8-3.9x real time), and tying it to the pass killed every encode at the ceiling and
// restarted from segment zero, so a cold store never converged.
//
// A candidate already in flight is skipped, so a slow publication is never started twice. While any
// worker is draining after a foreground preemption, launching pauses so the preempted (most urgent)
// work is re-admitted ahead of less urgent candidates on the next pass. The pool bounds admission:
// capacity minus the reserved foreground slot, and nothing while a live lease is held or waiting.
func (p *Planner) launch(ctx context.Context, candidates []Candidate) {
	for _, candidate := range candidates {
		if ctx.Err() != nil || p.lifecycle.Err() != nil {
			return
		}
		if p.draining() {
			return
		}
		if p.inFlight(candidate.Request) || p.coolingDown(candidate.Request) {
			continue
		}
		workCtx, release, ok := p.pool.AcquireBackground(p.lifecycle, candidate.NeededAt)
		if !ok {
			return
		}
		p.start(workCtx, release, candidate.Request)
	}
}

func (p *Planner) start(workCtx context.Context, release func(), request Request) {
	p.workerMu.Lock()
	p.workers[request] = workCtx
	p.workerMu.Unlock()
	p.workerWG.Add(1)
	go func() {
		defer p.workerWG.Done()
		// The slot is yielded the moment the lease is cancelled (a live session preempting), not
		// after this publication finishes tearing its staging workspace down: that cleanup holds no
		// encoder, and counting it against the live session's bounded wait sent viewers to software.
		stop := context.AfterFunc(workCtx, release)
		defer stop()
		_, err := p.preparer.Prepare(workCtx, request)
		preempted := p.lifecycle.Err() == nil && errors.Is(err, context.Canceled) && workCtx.Err() != nil
		release()
		p.workerMu.Lock()
		delete(p.workers, request)
		if err != nil && !preempted && p.lifecycle.Err() == nil {
			p.failures = append(p.failures, fmt.Errorf("prepare source %q: %w", request.Source.SourceID, err))
			p.failedAt[request] = p.now()
		}
		p.workerMu.Unlock()
		if err != nil && !preempted && p.lifecycle.Err() == nil {
			p.log.Warn("prepared media publication failed", "source", request.Source.SourceID, "err", err)
		}
	}()
}

func (p *Planner) inFlight(request Request) bool {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()
	_, ok := p.workers[request]
	return ok
}

// coolingDown reports whether request failed recently enough that retrying it now would only
// hold a scarce background slot for a source that is probably still broken. Without it, a corrupt
// file at the head of the queue would be restarted every tick and starve everything behind it.
func (p *Planner) coolingDown(request Request) bool {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()
	failed, ok := p.failedAt[request]
	if !ok {
		return false
	}
	if p.now().Sub(failed) >= preparationRetryBackoff {
		delete(p.failedAt, request)
		return false
	}
	return true
}

// draining reports whether any worker has been cancelled but has not yet exited.
func (p *Planner) draining() bool {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()
	for _, workCtx := range p.workers {
		if workCtx.Err() != nil {
			return true
		}
	}
	return false
}

// takeFailures returns the publication failures recorded since the previous pass so the pass that
// reports status carries them, even though the workers that failed ran between passes.
func (p *Planner) takeFailures() []error {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()
	failures := p.failures
	p.failures = nil
	return failures
}

// Wait blocks until every in-flight publication has exited or ctx ends. Shutdown cancels the
// lifecycle first, so workers observe cancellation, discard their private staging (Library.Publish
// removes its workspace on any failure, so no half-published entry is ever visible) and return.
func (p *Planner) Wait(ctx context.Context) error {
	if p == nil {
		return nil
	}
	drained := make(chan struct{})
	go func() { p.workerWG.Wait(); close(drained) }()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func uniqueCandidates(candidates []Candidate) []Candidate {
	candidates = append([]Candidate(nil), candidates...)
	slices.SortStableFunc(candidates, compareCandidates)
	seen := make(map[Request]struct{}, len(candidates))
	unique := candidates[:0]
	for _, candidate := range candidates {
		if _, duplicate := seen[candidate.Request]; duplicate {
			continue
		}
		seen[candidate.Request] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func compareCandidates(a, b Candidate) int {
	if a.Class < b.Class {
		return -1
	}
	if a.Class > b.Class {
		return 1
	}
	return a.NeededAt.Compare(b.NeededAt)
}

func retentionStatusFrom(result PruneResult) RetentionStatus {
	return RetentionStatus{
		RemainingBytes: result.RemainingBytes, BudgetBytes: result.BudgetBytes,
		ProtectedBytes: result.ProtectedBytes, PublicationsEvicted: result.PublicationsEvicted,
		BytesEvicted: result.BytesEvicted, StagingRemoved: result.StagingRemoved,
	}
}
