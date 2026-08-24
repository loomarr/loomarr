package filler_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestAcquisitionOutcomeFromUsesPipelineLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_900_000_000, 0).UTC()
	rows := []filler.ClipPipeline{
		{Status: filler.StatusQueued, Disposition: filler.DispositionRunning},
		{Status: filler.StatusRunning, Disposition: filler.DispositionRunning},
		{Status: filler.StatusFailed, Disposition: filler.DispositionRunning, NextRun: now.Add(time.Hour)},
		{Status: filler.StatusDone, Disposition: filler.DispositionReview},
		{Status: filler.StatusDone, Disposition: filler.DispositionFiled},
		{Status: filler.StatusFailed, Disposition: filler.DispositionRejected},
		{Status: filler.StatusDone, Disposition: filler.DispositionDismissed},
	}
	want := filler.AcquisitionOutcome{
		Enrolled: 7, Preparing: 3, NeedsDecision: 1, Admitted: 1, Rejected: 1, Dismissed: 1,
	}
	if got := filler.AcquisitionOutcomeFrom(rows, now); got != want {
		t.Fatalf("AcquisitionOutcomeFrom() = %+v, want %+v", got, want)
	}
}
