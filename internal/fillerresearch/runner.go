package fillerresearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Candidate struct {
	ClipHash      string
	Path          string
	Name          string
	SourceID      string
	InputRevision int64
	KnownEra      int
	KnownCountry  string
}

type Repository interface {
	ListCandidates(ctx context.Context, producer, producerVersion, adapter, adapterVersion string, limit int) ([]Candidate, error)
	SaveReport(ctx context.Context, report Report) error
}

type SignalLoader func(context.Context, Candidate) (Input, error)

type RunResult struct {
	Considered int
	Updated    int
	Failed     int
}

type Runner struct {
	repository Repository
	researcher *Researcher
	load       SignalLoader
	limit      func() int
}

func NewRunner(repository Repository, researcher *Researcher, load SignalLoader, limit func() int) *Runner {
	if limit == nil {
		limit = func() int { return 3 }
	}
	return &Runner{repository: repository, researcher: researcher, load: load, limit: limit}
}

func (r *Runner) Run(ctx context.Context) (RunResult, error) {
	var result RunResult
	if r == nil || r.repository == nil || r.researcher == nil || r.load == nil || r.limit() <= 0 {
		return result, nil
	}
	candidates, err := r.repository.ListCandidates(ctx, r.researcher.producer, r.researcher.version,
		"mediawiki", MediaWikiAdapterVersion, r.limit())
	if err != nil {
		return result, fmt.Errorf("list filler context candidates: %w", err)
	}
	result.Considered = len(candidates)
	var failures []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		input, err := r.load(ctx, candidate)
		if err != nil {
			return result, fmt.Errorf("load filler context signals for %s: %w", candidate.ClipHash, err)
		}
		input.ClipHash = candidate.ClipHash
		input.InputRevision = candidate.InputRevision
		input.SourceID = candidate.SourceID
		input.KnownEra = candidate.KnownEra
		input.KnownCountry = candidate.KnownCountry
		if input.Title == "" {
			input.Title = candidate.Name
		}
		if input.SourceKind == "" {
			input.SourceKind, _, _ = strings.Cut(candidate.SourceID, ":")
		}
		report, err := r.researcher.Research(ctx, input)
		if err != nil {
			result.Failed++
			failures = append(failures, fmt.Errorf("research filler context for %s: %w", candidate.ClipHash, err))
			continue
		}
		if err := r.repository.SaveReport(ctx, report); err != nil {
			result.Failed++
			failures = append(failures, fmt.Errorf("save filler context for %s: %w", candidate.ClipHash, err))
			continue
		}
		result.Updated++
	}
	return result, errors.Join(failures...)
}
