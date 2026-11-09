package filler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

type readyRepository struct {
	commits []filler.ReadyCommit
}

func (r *readyRepository) CommitFillerReady(_ context.Context, commit filler.ReadyCommit) error {
	r.commits = append(r.commits, commit)
	return nil
}

func completedReadyRow(hash string, at time.Time) pipelinestate.ClipPipeline {
	records := make([]pipelinestate.StageRecord, 0, len(pipelinestate.StageOrder))
	for _, stage := range pipelinestate.StageOrder {
		records = append(records, pipelinestate.StageRecord{Stage: stage, Status: pipelinestate.StatusDone, At: at})
	}
	return pipelinestate.ClipPipeline{
		ClipHash: hash, AcquisitionID: "acquisition-1", Stage: pipelinestate.StageFinish,
		Status: pipelinestate.StatusDone, Progress: 100, Disposition: pipelinestate.DispositionRunning,
		Stages: records, EnrolledAt: at, UpdatedAt: at,
	}
}

func readyClip(hash string) cliplibrary.StoreClip {
	return cliplibrary.StoreClip{Clip: clips.Clip{
		Hash: hash, Path: "clip.mp4", Source: "archive:classic_tv_commercials",
		Role: clips.Unclassified, Held: true, DurationMs: 30_000,
	}}
}

func TestTerminalReadyRequiresTheFinalCompletedLadder(t *testing.T) {
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	clip := readyClip("clip-hash")
	tests := []struct {
		name   string
		change func(*pipelinestate.ClipPipeline)
	}{
		{name: "not final stage", change: func(row *pipelinestate.ClipPipeline) { row.Stage = pipelinestate.StageVision }},
		{name: "final stage still running", change: func(row *pipelinestate.ClipPipeline) { row.Status = pipelinestate.StatusRunning }},
		{name: "missing required rung", change: func(row *pipelinestate.ClipPipeline) { row.Stages = row.Stages[1:] }},
		{name: "failed rung", change: func(row *pipelinestate.ClipPipeline) { row.Stages[0].Status = pipelinestate.StatusFailed }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &readyRepository{}
			row := completedReadyRow(clip.Hash, at)
			tt.change(&row)

			_, err := filler.NewTerminalReady(repo, func() time.Time { return at }).Commit(t.Context(), clip, row)
			if !errors.Is(err, filler.ErrReadyStale) {
				t.Fatalf("Commit error = %v, want ErrReadyStale", err)
			}
			if len(repo.commits) != 0 {
				t.Fatalf("invalid readiness reached repository: %+v", repo.commits)
			}
		})
	}
}

func TestTerminalReadyPublishesOnlyAfterEveryRungCompletesOrSkips(t *testing.T) {
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	clip := readyClip("clip-hash")
	row := completedReadyRow(clip.Hash, at)
	row.Stages[pipelinestate.StageIndex(pipelinestate.StageVision)].Status = pipelinestate.StatusSkipped
	repo := &readyRepository{}

	settled, err := filler.NewTerminalReady(repo, func() time.Time { return at }).Commit(t.Context(), clip, row)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.commits) != 1 || settled.Status != pipelinestate.StatusDone || settled.Progress != 100 {
		t.Fatalf("settled = %+v, commits = %+v", settled, repo.commits)
	}
}

func TestReadyCommitRejectsInvalidAuthorityAndPlacement(t *testing.T) {
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	row := completedReadyRow("clip-hash", at)
	row.Disposition = pipelinestate.DispositionReady
	valid := filler.ReadyCommit{Event: filler.ReadyEvent{
		ID: "ready:clip-hash", ClipHash: "clip-hash", AcquisitionID: "acquisition-1",
		Enrollment: filler.Enrollment{Kind: filler.EnrollmentAcquisition, Reference: "acquisition-1"},
		Placement:  clips.PlacementBreakBody, CreatedAt: at,
	}, Pipeline: row}
	tests := []struct {
		name   string
		change func(*filler.ReadyCommit)
	}{
		{name: "event id", change: func(c *filler.ReadyCommit) { c.Event.ID = "ready:other" }},
		{name: "unknown placement", change: func(c *filler.ReadyCommit) { c.Event.Placement = "sidecar" }},
		{name: "unknown enrollment", change: func(c *filler.ReadyCommit) { c.Event.Enrollment.Kind = "legacy" }},
		{name: "acquisition mismatch", change: func(c *filler.ReadyCommit) { c.Event.Enrollment.Reference = "other" }},
		{name: "source over acquisition", change: func(c *filler.ReadyCommit) {
			c.Event.Enrollment = filler.Enrollment{Kind: filler.EnrollmentSource, Reference: "archive:classic"}
		}},
		{name: "legacy ladder rung", change: func(c *filler.ReadyCommit) {
			c.Pipeline.Stages = append(c.Pipeline.Stages, pipelinestate.StageRecord{Stage: "admission", Status: pipelinestate.StatusDone, At: at})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commit := valid
			commit.Pipeline.Stages = append([]pipelinestate.StageRecord(nil), valid.Pipeline.Stages...)
			tt.change(&commit)
			if err := commit.Validate(); !errors.Is(err, filler.ErrReadyStale) {
				t.Fatalf("Validate error = %v, want ErrReadyStale", err)
			}
		})
	}
}

func TestReadyCommitRequiresExactTerminalLadderAdvance(t *testing.T) {
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	settled := completedReadyRow("clip-hash", at)
	settled.PreparationStartedAt = at.Add(250 * time.Millisecond)
	settled.StageQueuedAt = at.Add(350 * time.Millisecond)
	settled.StageStartedAt = at.Add(450 * time.Millisecond)
	settled.Disposition = pipelinestate.DispositionReady
	commit := filler.ReadyCommit{Event: filler.ReadyEvent{
		ID: "ready:clip-hash", ClipHash: "clip-hash", AcquisitionID: "acquisition-1",
		Enrollment: filler.Enrollment{Kind: filler.EnrollmentAcquisition, Reference: "acquisition-1"},
		Placement:  clips.PlacementBreakBody, CreatedAt: at,
	}, Pipeline: settled}
	current := settled
	current.Disposition = pipelinestate.DispositionRunning
	current.Status = pipelinestate.StatusRunning
	current.Progress = 0
	current.ForceRun = true // the successful terminal rung consumes this one-shot restart marker
	current.PreparationStartedAt = current.PreparationStartedAt.Truncate(time.Second)
	current.StageQueuedAt = current.StageQueuedAt.Truncate(time.Second)
	current.StageStartedAt = current.StageStartedAt.Truncate(time.Second)
	current.Stages = append([]pipelinestate.StageRecord(nil), settled.Stages[:len(settled.Stages)-1]...)
	if err := commit.ValidateAgainst(current); err != nil {
		t.Fatalf("exact terminal advance = %v", err)
	}
	stillForced := commit
	stillForced.Pipeline.ForceRun = true
	if err := stillForced.ValidateAgainst(current); !errors.Is(err, filler.ErrReadyStale) {
		t.Fatalf("terminal commit retaining force marker = %v, want ErrReadyStale", err)
	}

	tests := []struct {
		name   string
		change func(*pipelinestate.ClipPipeline)
	}{
		{name: "different attempt", change: func(row *pipelinestate.ClipPipeline) { row.Attempts++ }},
		{name: "changed prior record", change: func(row *pipelinestate.ClipPipeline) { row.Stages[0].Note = "changed" }},
		{name: "already terminal", change: func(row *pipelinestate.ClipPipeline) { row.Disposition = pipelinestate.DispositionReady }},
		{name: "different stage start second", change: func(row *pipelinestate.ClipPipeline) {
			row.StageStartedAt = row.StageStartedAt.Add(time.Second)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drifted := current
			drifted.Stages = append([]pipelinestate.StageRecord(nil), current.Stages...)
			tt.change(&drifted)
			if err := commit.ValidateAgainst(drifted); !errors.Is(err, filler.ErrReadyStale) {
				t.Fatalf("ValidateAgainst error = %v, want ErrReadyStale", err)
			}
		})
	}
}
