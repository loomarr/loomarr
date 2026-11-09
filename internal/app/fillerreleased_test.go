package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// passStage is a rung that runs and lets the clip continue.
type passStage struct{ id pipelinestate.StageID }

func (s passStage) ID() pipelinestate.StageID                              { return s.id }
func (passStage) Applies(context.Context, filler.StoreClip) (bool, string) { return true, "" }
func (passStage) Cost() filler.StageCost                                   { return filler.CostCheap }
func (passStage) Run(context.Context, filler.StoreClip) (filler.StageResult, error) {
	return filler.StageResult{Verdict: pipelinestate.VerdictContinue}, nil
}

// releasedClipRunningRow seeds the production shape from #1445: a clip an older path already
// published (held=0, not removed, break_body) whose conveyor row was never settled and still sits
// running at the score rung with every earlier rung complete.
func releasedClipRunningRow(t *testing.T, st records.Store, hash string) {
	t.Helper()
	ctx := context.Background()
	at := time.Now().UTC().Add(-time.Hour)
	if err := st.UpsertClip(ctx, clipstore.Clip{Clip: filler.Clip{
		Hash: hash, Path: "ab/" + hash + ".mp4", Name: hash + ".mp4", Role: filler.Commercial,
		DurationMs: 30_000, Source: "archive", Placement: filler.PlacementBreakBody,
	}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	var stages []pipelinestate.StageRecord
	for _, id := range pipelinestate.StageOrder[:len(pipelinestate.StageOrder)-1] {
		stages = append(stages, pipelinestate.StageRecord{Stage: id, Status: pipelinestate.StatusDone, At: at})
	}
	if err := st.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
		ClipHash: hash, Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusRunning,
		Disposition: pipelinestate.DispositionRunning, Stages: stages, Attempts: 3,
		EnrolledAt: at, UpdatedAt: at, StageQueuedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func releasedClipPipeline(st records.Store) *filler.Pipeline {
	stages := make([]filler.Stage, 0, len(pipelinestate.StageOrder))
	for _, id := range pipelinestate.StageOrder {
		stages = append(stages, passStage{id: id})
	}
	return filler.NewPipeline(st, fillerPipelineClipAdapter{st}, stages, testFillerBudget(), nil, time.Now, nil)
}

// testFillerBudget is the historical per-pass batch sizes; production reads them from settings.
func testFillerBudget() filler.Budget {
	return filler.Budget{
		MaxClips:      func() int { return 25 },
		MaxTranscodes: func() int { return 3 },
		MaxWhisper:    func() int { return 10 },
		MaxVision:     func() int { return 5 },
		MaxSplits:     func() int { return 3 },
	}
}

func TestPipeline_RunningRowForAlreadyReleasedClipSettlesWithoutRepublishing(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	releasedClipRunningRow(t, st, "released")
	before, err := st.GetClip(ctx, "released")
	if err != nil {
		t.Fatal(err)
	}
	if before.Held || before.Placement != filler.PlacementBreakBody {
		t.Fatalf("seed is not the released shape: held=%v placement=%q", before.Held, before.Placement)
	}

	res, err := releasedClipPipeline(st).RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 0 {
		t.Fatalf("a released clip's row failed publication instead of settling: %+v", res)
	}
	row, found, err := st.GetClipPipeline(ctx, "released")
	if err != nil || !found {
		t.Fatalf("row missing: found=%v err=%v", found, err)
	}
	if row.Disposition != pipelinestate.DispositionReady || row.Status != pipelinestate.StatusDone || !row.NextRun.IsZero() {
		t.Fatalf("row did not settle: %+v", row)
	}
	if note := row.Stages[pipelinestate.StageIndex(pipelinestate.StageFinish)].Note; note == "" {
		t.Fatal("the settlement left no note on the score rung")
	}
	after, err := st.GetClip(ctx, "released")
	if err != nil {
		t.Fatal(err)
	}
	if after.Held || after.Role != before.Role || after.Placement != before.Placement {
		t.Fatalf("settling changed the clip: before=%+v after=%+v", before, after)
	}
	if _, ok, err := st.GetFillerReadyEvent(ctx, "released"); err != nil || ok {
		t.Fatalf("settling republished the clip: ready event found=%v err=%v", ok, err)
	}
}

func TestPipeline_RunningRowForRemovedClipStillFailsStale(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	releasedClipRunningRow(t, st, "removed")
	if _, err := st.SetClipsRemoved(ctx, []string{"ab/removed.mp4"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	res, err := releasedClipPipeline(st).RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row, _, _ := st.GetClipPipeline(ctx, "removed")
	if res.Failed == 0 || row.Disposition == pipelinestate.DispositionReady {
		t.Fatalf("a removed clip must not settle as ready: res=%+v row=%+v", res, row)
	}
}

func TestTerminalReady_HashMismatchStillFailsStale(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	releasedClipRunningRow(t, st, "mismatch")
	clip, found, err := fillerPipelineClipAdapter{st}.GetClip(ctx, "mismatch")
	if err != nil || !found {
		t.Fatalf("clip missing: found=%v err=%v", found, err)
	}
	row, _, _ := st.GetClipPipeline(ctx, "mismatch")
	row.ClipHash = "someone-else"
	clip.Held = true
	if _, err := filler.NewTerminalReady(st, nil).Commit(ctx, clip, row); !errors.Is(err, filler.ErrReadyStale) {
		t.Fatalf("hash mismatch must stay stale, got %v", err)
	}
}
