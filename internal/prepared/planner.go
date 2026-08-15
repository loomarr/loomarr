package prepared

import (
	"context"
	"errors"
	"fmt"
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

type preparation interface {
	Prepare(context.Context, Request) (Publication, error)
}

// Planner is the readiness control plane. It can publish media but cannot mutate a schedule, and
// every encode runs under the shared preemptible background lease.
type Planner struct {
	resolver CandidateResolver
	preparer preparation
	pool     *media.EncodePool
	now      func() time.Time
}

func NewPlanner(
	resolver CandidateResolver, preparer preparation, pool *media.EncodePool, now func() time.Time,
) *Planner {
	if now == nil {
		now = time.Now
	}
	return &Planner{resolver: resolver, preparer: preparer, pool: pool, now: now}
}

// Run prepares as much of the next schedule window as spare hardware permits. A foreground
// preemption or a busy pool is a normal yield, not a failed task. Independent source failures are
// joined after the pass so one corrupt file cannot starve every later candidate.
func (p *Planner) Run(ctx context.Context) error {
	if p == nil || p.resolver == nil || p.preparer == nil || p.pool == nil {
		return nil
	}
	now := p.now()
	candidates, resolveErr := p.resolver.Candidates(ctx, now, now.Add(preparationLookahead))
	slices.SortStableFunc(candidates, func(a, b Candidate) int { return a.NeededAt.Compare(b.NeededAt) })
	seen := make(map[Request]struct{}, len(candidates))
	errs := []error{resolveErr}
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
	return errors.Join(errs...)
}
