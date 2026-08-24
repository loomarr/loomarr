package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

type readinessStoreStub struct {
	pipeline filler.PipelineOverview
	runs     []filler.AcquisitionRun
}

func (s readinessStoreStub) PipelineOverview(context.Context, time.Time) (filler.PipelineOverview, error) {
	return s.pipeline, nil
}

func (s readinessStoreStub) ListAcquisitionRuns(context.Context, int, time.Time) ([]filler.AcquisitionRun, error) {
	return s.runs, nil
}

func TestFillerReadinessComposesAuthoritativeServerFacts(t *testing.T) {
	poolAsked := false
	a := fillerServiceAdapter{
		readiness: readinessStoreStub{pipeline: filler.PipelineOverview{Rejected: 2, Recoverable: 2}},
		pool: func(context.Context) (filler.PoolReport, error) {
			poolAsked = true
			return filler.PoolReport{Eligible: 12}, nil
		},
		now: func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) },
	}

	got, err := a.Readiness(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// With no automatic fetcher configured, repairing acquisition is more important than the
	// rejected work behind it. This proves the adapter delegates the cross-domain priority rather
	// than exposing unrelated counters for a client to sort.
	if got.Next != filler.ReadinessEnableFetch {
		t.Fatalf("next = %q, want enable_fetch", got.Next)
	}
	if !poolAsked {
		t.Fatal("readiness did not include the live pool projection")
	}
}
