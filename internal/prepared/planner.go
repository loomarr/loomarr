package prepared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/media"
)

const preparationLookahead = 6 * time.Hour

// Candidate is one immutable source/rendition needed by the accepted schedule. NeededAt controls
// priority only; Channel identity is deliberately absent because publications are shared.
type Candidate struct {
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

// CandidateResolver reads the authoritative schedule and returns locally readable sources.
// Implemented at composition, where channels, library path mapping, and audio selection meet.
type CandidateResolver interface {
	Plan(context.Context, time.Time, time.Time) (ReadinessPlan, error)
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
// retention intentionally share one scheduler pass so lifecycle cannot drift into a bolt-on task.
type PlannerDependencies struct {
	Resolver          CandidateResolver
	Preparation       Preparation
	Pool              *media.EncodePool
	Retainer          Retainer
	BudgetBytes       func() int64
	Now               func() time.Time
	Log               *slog.Logger
	UnavailableReason string
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
	statusMu sync.RWMutex
	status   PlannerStatus
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
	return &Planner{
		resolver: deps.Resolver, preparer: deps.Preparation, pool: deps.Pool,
		retainer: deps.Retainer, budget: deps.BudgetBytes, now: deps.Now, log: deps.Log,
		status: PlannerStatus{Available: available, UnavailableReason: reason},
	}
}

// Status returns the latest immutable operational snapshot without touching the schedule or disk.
func (p *Planner) Status() PlannerStatus {
	if p == nil {
		return PlannerStatus{UnavailableReason: "the prepared playout planner is not wired"}
	}
	p.statusMu.RLock()
	defer p.statusMu.RUnlock()
	return p.status
}

// Run prepares as much of the next schedule window as spare hardware permits. A foreground
// preemption or a busy pool is a normal yield, not a failed task. Independent source failures are
// joined after the pass so one corrupt file cannot starve every later candidate.
func (p *Planner) Run(ctx context.Context) (runErr error) {
	if p == nil {
		return nil
	}
	p.statusMu.Lock()
	p.status.Running = true
	p.statusMu.Unlock()
	var errs []error
	var plan ReadinessPlan
	var retention PruneResult
	defer func() {
		p.statusMu.Lock()
		p.status.Running = false
		p.status.LastRunAt = p.now()
		p.status.Readiness = plan.Summary
		p.status.Retention = retentionStatusFrom(retention)
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
		candidates := plan.Candidates
		slices.SortStableFunc(candidates, func(a, b Candidate) int { return a.NeededAt.Compare(b.NeededAt) })
		seen := make(map[Request]struct{}, len(candidates))
		errs = append(errs, resolveErr)
		for _, candidate := range candidates {
			if _, duplicate := seen[candidate.Request]; duplicate {
				continue
			}
			seen[candidate.Request] = struct{}{}
			workCtx, release, ok := p.pool.AcquireBackground(ctx)
			if !ok {
				break
			}
			_, err := p.preparer.Prepare(workCtx, candidate.Request)
			preempted := ctx.Err() == nil && errors.Is(err, context.Canceled) && workCtx.Err() != nil
			release()
			if preempted {
				break
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("prepare %q: %w", candidate.Request.Source.Path, err))
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	if p.retainer != nil && p.budget != nil {
		budget := p.budget()
		result, err := p.retainer.Prune(ctx, budget, plan.Protected)
		retention = result
		if err != nil {
			errs = append(errs, fmt.Errorf("retain prepared media: %w", err))
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
	}
	runErr = errors.Join(errs...)
	return runErr
}

func retentionStatusFrom(result PruneResult) RetentionStatus {
	return RetentionStatus{
		RemainingBytes: result.RemainingBytes, BudgetBytes: result.BudgetBytes,
		ProtectedBytes: result.ProtectedBytes, PublicationsEvicted: result.PublicationsEvicted,
		BytesEvicted: result.BytesEvicted, StagingRemoved: result.StagingRemoved,
	}
}
