package acquisition_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

func TestAcquisitionOutcomeFromUsesPipelineLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_900_000_000, 0).UTC()
	rows := []pipelinestate.ClipPipeline{
		{Status: pipelinestate.StatusQueued, Disposition: pipelinestate.DispositionRunning},
		{Status: pipelinestate.StatusRunning, Disposition: pipelinestate.DispositionRunning},
		{Status: pipelinestate.StatusFailed, Disposition: pipelinestate.DispositionRunning, NextRun: now.Add(time.Hour)},
		{Status: pipelinestate.StatusDone, Disposition: pipelinestate.DispositionReview},
		{Status: pipelinestate.StatusDone, Disposition: pipelinestate.DispositionReady},
		{Status: pipelinestate.StatusDone, Disposition: pipelinestate.DispositionComplete},
		{Status: pipelinestate.StatusFailed, Disposition: pipelinestate.DispositionRejected},
		{Status: pipelinestate.StatusDone, Disposition: pipelinestate.DispositionDismissed},
	}
	want := acquisition.Outcome{
		Enrolled: 8, Preparing: 3, NeedsDecision: 1, Ready: 1, Complete: 1, Rejected: 1, Dismissed: 1,
	}
	if got := acquisition.OutcomeFrom(rows, now); got != want {
		t.Fatalf("AcquisitionOutcomeFrom() = %+v, want %+v", got, want)
	}
}
