package routine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

func TestFillerPipelineDriverRunsDetailsAfterPreparation(t *testing.T) {
	var order []string
	driver := PipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			order = append(order, "prepare")
			return filler.PipelineResult{}, errors.New("preparation failed")
		},
		details: func(context.Context) error {
			order = append(order, "details")
			return errors.New("details failed")
		},
	}
	err := driver.Run(t.Context())
	if strings.Join(order, ",") != "prepare,details" {
		t.Fatalf("order = %v", order)
	}
	if err == nil || !strings.Contains(err.Error(), "preparation failed") || !strings.Contains(err.Error(), "details failed") {
		t.Fatalf("joined error = %v", err)
	}
}

func TestFillerPipelineDriverStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	detailsRan := false
	driver := PipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			cancel()
			return filler.PipelineResult{}, nil
		},
		details: func(context.Context) error { detailsRan = true; return nil },
	}
	if err := driver.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if detailsRan {
		t.Fatal("details ran after the scheduler lease was cancelled")
	}
}

func TestFillerPipelineDriverDefersDetailsWhilePreparationCanAdvance(t *testing.T) {
	detailsRan := false
	driver := PipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			return filler.PipelineResult{Advanced: 1, Overview: pipelinestate.PipelineOverview{Runnable: 4}}, nil
		},
		details: func(context.Context) error {
			detailsRan = true
			return nil
		},
	}
	if err := driver.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if detailsRan {
		t.Fatal("optional details ran while playable preparation still had runnable clips")
	}
}

// #1412: production held 202 clips of which only 34 were ready. The rest sat Runnable/InProgress
// without the pipeline advancing them, and "defer details while anything is runnable" meant the
// enrichment runner never ran and filler_enrichment_passes stayed at zero rows.
func TestFillerPipelineDriverRunsDetailsWhenPreparationMakesNoProgress(t *testing.T) {
	detailsRan := false
	driver := PipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			return filler.PipelineResult{Overview: pipelinestate.PipelineOverview{Runnable: 100, InProgress: 3}}, nil
		},
		details: func(context.Context) error { detailsRan = true; return nil },
	}
	if err := driver.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !detailsRan {
		t.Fatal("details starved behind a backlog that preparation did not advance")
	}
}

func TestFillerPipelineDriverBoundsDetailDeferralWhilePreparationAdvances(t *testing.T) {
	ran := 0
	driver := &PipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			return filler.PipelineResult{Advanced: 1, Overview: pipelinestate.PipelineOverview{Runnable: 100}}, nil
		},
		details: func(context.Context) error { ran++; return nil },
	}
	for range detailMaxDeferrals + 1 {
		if err := driver.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if ran != 1 {
		t.Fatalf("details ran %d times over %d advancing passes, want exactly once after the deferral bound", ran, detailMaxDeferrals+1)
	}
}
