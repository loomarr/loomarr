package fillerenrichment

import (
	"context"
	"fmt"
	"time"
)

type Candidate struct {
	ClipHash    string
	Path        string
	Name        string
	Kind        string
	Transcript  string
	VisibleText string
}

type Repository interface {
	ListCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]Candidate, error)
	ApplyPass(ctx context.Context, pass Pass) (int, error)
}

type SignalLoader func(context.Context, Candidate, time.Time) (Signals, error)

type Runner struct {
	repository Repository
	load       SignalLoader
	limit      func() int
	now        func() time.Time
}

func NewRunner(repository Repository, load SignalLoader, limit func() int, now func() time.Time) *Runner {
	if limit == nil {
		limit = func() int { return 10 }
	}
	if now == nil {
		now = time.Now
	}
	return &Runner{repository: repository, load: load, limit: limit, now: now}
}

type RunResult struct {
	Considered int
	Updated    int
	Failed     int
}

// Run performs one bounded, provider-free pass. Missing sidecars are handled by the loader as
// ordinary sparse signals; persistence failures stop the pass so the un-stamped clip retries.
func (r *Runner) Run(ctx context.Context) (RunResult, error) {
	if r == nil || r.repository == nil || r.load == nil {
		return RunResult{}, nil
	}
	limit := r.limit()
	if limit <= 0 {
		return RunResult{}, nil
	}
	candidates, err := r.repository.ListCandidates(ctx, DeterministicProducer, DeterministicProducerVersion, ControlledTaxonomyVersion, limit)
	if err != nil {
		return RunResult{}, fmt.Errorf("list deterministic enrichment candidates: %w", err)
	}
	result := RunResult{Considered: len(candidates)}
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		completedAt := r.now().UTC()
		signals, err := r.load(ctx, candidate, completedAt)
		if err != nil {
			return result, fmt.Errorf("load enrichment signals for %s: %w", candidate.ClipHash, err)
		}
		signals.ClipHash = candidate.ClipHash
		if signals.Kind == "" {
			signals.Kind = candidate.Kind
		}
		if signals.Title == "" {
			signals.Title = candidate.Name
		}
		signals.ObservedAt = completedAt
		changed, err := r.repository.ApplyPass(ctx, Pass{
			ClipHash: candidate.ClipHash, Producer: DeterministicProducer,
			ProducerVersion: DeterministicProducerVersion, TaxonomyVersion: ControlledTaxonomyVersion,
			CompletedAt: completedAt, States: AnalyzeDeterministic(signals),
		})
		if err != nil {
			return result, fmt.Errorf("apply deterministic enrichment for %s: %w", candidate.ClipHash, err)
		}
		result.Updated += changed
	}
	return result, nil
}
