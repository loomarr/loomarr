package prepared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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

// CandidateResolver reads the authoritative schedule and returns locally readable sources.
// Implemented at composition, where channels, library path mapping, and audio selection meet.
type CandidateResolver interface {
	Candidates(context.Context, time.Time, time.Time) ([]Candidate, error)
}

// Preparation publishes one request. Preparer implements it; the interface keeps Planner focused
// on schedule priority rather than fingerprinting or packaging internals.
type Preparation interface {
	Prepare(context.Context, Request) (Publication, error)
}

// Retainer owns the lifecycle of the same immutable store preparation writes.
type Retainer interface {
	Prune(context.Context, int64) (PruneResult, error)
}

// PlannerDependencies makes the readiness control plane's ownership explicit. Preparation and
// retention intentionally share one scheduler pass so lifecycle cannot drift into a bolt-on task.
type PlannerDependencies struct {
	Resolver    CandidateResolver
	Preparation Preparation
	Pool        *media.EncodePool
	Retainer    Retainer
	BudgetBytes func() int64
	Now         func() time.Time
	Log         *slog.Logger
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
}

func NewPlanner(deps PlannerDependencies) *Planner {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Log == nil {
		deps.Log = slog.New(slog.DiscardHandler)
	}
	return &Planner{
		resolver: deps.Resolver, preparer: deps.Preparation, pool: deps.Pool,
		retainer: deps.Retainer, budget: deps.BudgetBytes, now: deps.Now, log: deps.Log,
	}
}

// Run prepares as much of the next schedule window as spare hardware permits. A foreground
// preemption or a busy pool is a normal yield, not a failed task. Independent source failures are
// joined after the pass so one corrupt file cannot starve every later candidate.
func (p *Planner) Run(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.resolver != nil && p.preparer != nil && p.pool != nil {
		now := p.now()
		candidates, resolveErr := p.resolver.Candidates(ctx, now, now.Add(preparationLookahead))
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
		result, err := p.retainer.Prune(ctx, budget)
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
	return errors.Join(errs...)
}
