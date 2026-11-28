package screening_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/screening"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The screen stage is staged: no install wires it. Running it through the pipeline keeps it fit to
// be wired when the filler.Stage contract or the pipeline changes.
func TestSegmentScreeningStageRunsThroughThePipeline(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC()
	clip, _, root := screening.RenderedChildStageFixture(t)
	clip.DurationMs = 30_000
	runtime, _, _ := screening.ScreeningStageRuntime(t, screening.ScreenPass)

	st := testkit.MigratedSQLiteStore(t)
	if err := st.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
		ClipHash: clip.Hash, Stage: pipelinestate.StageScreen, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionRunning, NextRun: now, EnrolledAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	pipeline := filler.NewPipeline(st, storeadapter.NewPipelineClips(st),
		[]filler.Stage{screening.NewSegmentScreeningStage(runtime, nil, root)},
		filler.Budget{}, nil, func() time.Time { return now }, nil)
	if _, err := pipeline.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	row, found, err := st.GetClipPipeline(ctx, clip.Hash)
	if err != nil || !found {
		t.Fatalf("pipeline row: found=%v err=%v", found, err)
	}
	if row.Stage != pipelinestate.StageScreen || row.Disposition != pipelinestate.DispositionReview {
		t.Fatalf("child was not held at the screen stage: stage=%s disposition=%s", row.Stage, row.Disposition)
	}
	const want = "rendered-child screening passed but production release is not authorized"
	for _, record := range row.Stages {
		if record.Stage == pipelinestate.StageScreen {
			if record.Note != want {
				t.Fatalf("screen record = %+v, want note %q", record, want)
			}
			return
		}
	}
	t.Fatalf("no screen record on the ladder: %+v", row.Stages)
}
