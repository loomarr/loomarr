package filler_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// A due running row whose clip was removed can never publish, so the pass retires it, and the clip
// keeps the time it was removed.
func TestPipeline_ARowWhoseClipWasRemovedRetires(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "c1")
	removedAt := time.Unix(1_799_000_000, 0).UTC()
	clip := st.clips["c1"]
	clip.RemovedAt = removedAt
	st.clips["c1"] = clip

	if _, err := newPipe(st, nil, filler.DefaultBudget()).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.rows["c1"]
	if got.Disposition != pipelinestate.DispositionRejected || !got.NextRun.IsZero() {
		t.Fatalf("row = %q next_run=%v, want rejected and unscheduled", got.Disposition, got.NextRun)
	}
	if !st.clips["c1"].RemovedAt.Equal(removedAt) {
		t.Errorf("removed_at = %v, want it kept at %v", st.clips["c1"].RemovedAt, removedAt)
	}
}
