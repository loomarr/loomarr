package filler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// legacyScoreRow is the production shape from #1392: a non-segment row that finished its ladder
// before StageScreen existed, so it holds seven records and no screen, parked at score/running.
func legacyScoreRow(hash string) filler.ClipPipeline {
	at := time.Unix(1_799_999_000, 0).UTC()
	var stages []filler.StageRecord
	for _, id := range filler.StageOrder {
		if id == filler.StageScreen {
			continue
		}
		stages = append(stages, filler.StageRecord{Stage: id, Status: filler.StatusDone, At: at})
	}
	return filler.ClipPipeline{
		ClipHash: hash, Stage: filler.StageScore, Status: filler.StatusRunning,
		Disposition: filler.DispositionRunning, Attempts: 2075, Stages: stages,
		EnrolledAt: at, UpdatedAt: at, StageQueuedAt: at,
		PreparationAttempt: 1, PreparationStartedAt: at,
		PreparationStartReason: filler.PreparationStartedByEnrollment,
	}
}

func TestPipeline_LegacyRowMissingScreenRecordIsRewoundAndReachesReady(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "legacy")
	st.rows["legacy"] = legacyScoreRow("legacy")
	stages := allStages()
	stages[filler.StageScreen].applies = false
	stages[filler.StageScreen].note = "not a rendered child"

	p := newPipe(st, asSlice(stages), filler.DefaultBudget())
	res, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := st.rows["legacy"]
	if res.Failed != 0 || row.Disposition != filler.DispositionReady || len(row.Stages) != len(filler.StageOrder) {
		t.Fatalf("legacy row did not reach ready: result=%+v row=%+v", res, row)
	}
	for i, id := range filler.StageOrder {
		if row.Stages[i].Stage != id {
			t.Fatalf("stage %d = %s, want %s (ladder out of order): %+v", i, row.Stages[i].Stage, id, row.Stages)
		}
	}
	if got := row.Stages[filler.StageIndex(filler.StageScreen)].Status; got != filler.StatusSkipped {
		t.Fatalf("screen record = %s, want skipped for a non-segment", got)
	}
	if row.Attempts > 2 {
		t.Fatalf("attempts = %d; the legacy pass count must not carry into the rewound rungs", row.Attempts)
	}
}

func TestPipeline_LegacyRowMissingEarlierRungsRewindsToTheFirstGap(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "short")
	row := legacyScoreRow("short")
	row.Stages = row.Stages[:2] // probe, transcode only
	st.rows["short"] = row
	stages := allStages()

	p := newPipe(st, asSlice(stages), filler.DefaultBudget())
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.rows["short"]
	if got.Disposition != filler.DispositionReady || stages[filler.StageSplit].runs != 1 || stages[filler.StageProbe].runs != 0 {
		t.Fatalf("did not resume at the first missing rung: row=%+v split=%d probe=%d",
			got, stages[filler.StageSplit].runs, stages[filler.StageProbe].runs)
	}
}

// A publication error must take the ordinary failure path: backoff, persisted, bounded attempts —
// and it must stop occupying a work slot so due rows behind it are processed.
func TestPipeline_ReadyCommitFailureBacksOffAndYieldsItsSlot(t *testing.T) {
	st := newPipeMemStore()
	st.commitErr = errors.New("catalog write failed")
	seedEnrolled(st, "poison")
	poison := st.rows["poison"]
	for _, id := range filler.StageOrder {
		poison.Stages = append(poison.Stages, filler.StageRecord{Stage: id, Status: filler.StatusDone})
	}
	poison.Stage, poison.Status = filler.StageScore, filler.StatusQueued
	st.rows["poison"] = poison

	p := newPipe(st, asSlice(allStages()), filler.DefaultBudget())
	for range 10 {
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	got := st.rows["poison"]
	if got.Disposition != filler.DispositionRunning || got.Attempts > filler.MaxAttempts {
		t.Fatalf("attempts grew without bound or row left running state: %+v", got)
	}
	if !got.NextRun.After(time.Unix(1_800_000_000, 0)) {
		t.Fatalf("failing commit was not backed off: next_run=%v", got.NextRun)
	}

	// A due row behind it is processed even though the poisoned row failed.
	seedEnrolled(st, "due")
	st.commitErr = nil
	res, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.rows["due"].Disposition != filler.DispositionReady || res.Failed != 0 {
		t.Fatalf("due row starved behind the failing commit: result=%+v row=%+v", res, st.rows["due"])
	}
}

// A commit that fails once must not leave the failed score record behind: after the backoff the
// score rung runs again, records done, and the row publishes with a full ordered ladder.
func TestPipeline_ReadyCommitFailingOnceRecoversToReadyAfterBackoff(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "once")
	row := st.rows["once"]
	for _, id := range filler.StageOrder {
		row.Stages = append(row.Stages, filler.StageRecord{Stage: id, Status: filler.StatusDone})
	}
	row.Stage, row.Status = filler.StageScore, filler.StatusQueued
	st.rows["once"] = row

	now := time.Unix(1_800_000_000, 0).UTC()
	p := filler.NewPipeline(st, st, asSlice(allStages()), filler.DefaultBudget(), nil, func() time.Time { return now }, nil)

	st.commitErr = errors.New("catalog write failed")
	res, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	failed := st.rows["once"]
	if res.Failed != 1 || failed.Status != filler.StatusFailed || !failed.NextRun.After(now) {
		t.Fatalf("first pass = %+v row=%+v, want one failure with a future next_run", res, failed)
	}

	st.commitErr = nil
	now = failed.NextRun.Add(time.Second)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := st.rows["once"]
	if got.Disposition != filler.DispositionReady || got.Status != filler.StatusDone || len(got.Stages) != len(filler.StageOrder) {
		t.Fatalf("row did not recover to ready: %+v", got)
	}
	for i, id := range filler.StageOrder {
		if got.Stages[i].Stage != id || (got.Stages[i].Status != filler.StatusDone && got.Stages[i].Status != filler.StatusSkipped) {
			t.Fatalf("ladder[%d] = %+v, want %s complete", i, got.Stages[i], id)
		}
	}
	if score := got.Stages[filler.StageIndex(filler.StageScore)]; score.Status != filler.StatusDone || got.Attempts > filler.MaxAttempts {
		t.Fatalf("score=%+v attempts=%d, want score done and attempts within MaxAttempts", score, got.Attempts)
	}
}
