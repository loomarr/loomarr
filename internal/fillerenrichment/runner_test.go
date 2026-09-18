package fillerenrichment_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

type runnerRepository struct {
	candidates []fillerenrichment.Candidate
	passes     []fillerenrichment.Pass
}

func (r *runnerRepository) ListCandidates(context.Context, string, string, string, int) ([]fillerenrichment.Candidate, error) {
	return r.candidates, nil
}
func (r *runnerRepository) ApplyPass(_ context.Context, pass fillerenrichment.Pass) (int, error) {
	r.passes = append(r.passes, pass)
	return len(pass.States), nil
}

func TestRunner_BoundsAndStampsTheDeterministicPass(t *testing.T) {
	repository := &runnerRepository{candidates: []fillerenrichment.Candidate{{ClipHash: "hp", Path: "hp.mp4", Name: "HP Sauce Advert", Kind: "commercial"}}}
	now := time.Unix(100, 0).UTC()
	runner := fillerenrichment.NewRunner(repository, func(_ context.Context, candidate fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{Description: "An advert for HP Sauce broadcast 1999 on Five."}, nil
	}, func() int { return 10 }, func() time.Time { return now })
	result, err := runner.Run(context.Background())
	if err != nil || result.Considered != 1 || result.Updated != len(fillerenrichment.DeterministicAxes) {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	if len(repository.passes) != 1 {
		t.Fatalf("passes = %d", len(repository.passes))
	}
	pass := repository.passes[0]
	if pass.ClipHash != "hp" || pass.Producer != fillerenrichment.DeterministicProducer ||
		pass.ProducerVersion != fillerenrichment.DeterministicProducerVersion || !pass.CompletedAt.Equal(now) {
		t.Fatalf("pass = %+v", pass)
	}
}
