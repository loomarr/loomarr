package filler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

type readyRepository struct {
	commits []filler.ReadyCommit
}

func (r *readyRepository) CommitFillerReady(_ context.Context, commit filler.ReadyCommit) error {
	r.commits = append(r.commits, commit)
	return nil
}

func completedReadyRow(hash string, at time.Time) filler.ClipPipeline {
	records := make([]filler.StageRecord, 0, len(filler.StageOrder))
	for _, stage := range filler.StageOrder {
		records = append(records, filler.StageRecord{Stage: stage, Status: filler.StatusDone, At: at})
	}
	return filler.ClipPipeline{
		ClipHash: hash, AcquisitionID: "acquisition-1", Stage: filler.StageScore,
		Status: filler.StatusDone, Progress: 100, Disposition: filler.DispositionRunning,
		Stages: records, EnrolledAt: at, UpdatedAt: at,
	}
}

func readyClip(hash string) filler.StoreClip {
	return filler.StoreClip{Clip: filler.Clip{
		Hash: hash, Path: "clip.mp4", Source: "archive:classic_tv_commercials",
		Kind: filler.Unclassified, Held: true, DurationMs: 30_000,
	}}
}

func TestTerminalReadyRequiresTheFinalCompletedLadder(t *testing.T) {
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	clip := readyClip("clip-hash")
	tests := []struct {
		name   string
		change func(*filler.ClipPipeline)
	}{
		{name: "not final stage", change: func(row *filler.ClipPipeline) { row.Stage = filler.StageVision }},
		{name: "final stage still running", change: func(row *filler.ClipPipeline) { row.Status = filler.StatusRunning }},
		{name: "missing required rung", change: func(row *filler.ClipPipeline) { row.Stages = row.Stages[1:] }},
		{name: "failed rung", change: func(row *filler.ClipPipeline) { row.Stages[0].Status = filler.StatusFailed }},
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
	row.Stages[filler.StageIndex(filler.StageVision)].Status = filler.StatusSkipped
	repo := &readyRepository{}

	settled, err := filler.NewTerminalReady(repo, func() time.Time { return at }).Commit(t.Context(), clip, row)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.commits) != 1 || settled.Status != filler.StatusDone || settled.Progress != 100 {
		t.Fatalf("settled = %+v, commits = %+v", settled, repo.commits)
	}
}

func TestReadyCommitRejectsInvalidAuthorityAndPlacement(t *testing.T) {
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	row := completedReadyRow("clip-hash", at)
	row.Disposition = filler.DispositionReady
	valid := filler.ReadyCommit{Event: filler.ReadyEvent{
		ID: "ready:clip-hash", ClipHash: "clip-hash", AcquisitionID: "acquisition-1",
		Enrollment: filler.Enrollment{Kind: filler.EnrollmentAcquisition, Reference: "acquisition-1"},
		Placement:  filler.PlacementBreakBody, CreatedAt: at,
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
			c.Pipeline.Stages = append(c.Pipeline.Stages, filler.StageRecord{Stage: "admission", Status: filler.StatusDone, At: at})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commit := valid
			commit.Pipeline.Stages = append([]filler.StageRecord(nil), valid.Pipeline.Stages...)
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
	settled.Disposition = filler.DispositionReady
	commit := filler.ReadyCommit{Event: filler.ReadyEvent{
		ID: "ready:clip-hash", ClipHash: "clip-hash", AcquisitionID: "acquisition-1",
		Enrollment: filler.Enrollment{Kind: filler.EnrollmentAcquisition, Reference: "acquisition-1"},
		Placement:  filler.PlacementBreakBody, CreatedAt: at,
	}, Pipeline: settled}
	current := settled
	current.Disposition = filler.DispositionRunning
	current.Status = filler.StatusRunning
	current.Progress = 0
	current.PreparationStartedAt = current.PreparationStartedAt.Truncate(time.Second)
	current.StageQueuedAt = current.StageQueuedAt.Truncate(time.Second)
	current.StageStartedAt = current.StageStartedAt.Truncate(time.Second)
	current.Stages = append([]filler.StageRecord(nil), settled.Stages[:len(settled.Stages)-1]...)
	if err := commit.ValidateAgainst(current); err != nil {
		t.Fatalf("exact terminal advance = %v", err)
	}

	tests := []struct {
		name   string
		change func(*filler.ClipPipeline)
	}{
		{name: "different attempt", change: func(row *filler.ClipPipeline) { row.Attempts++ }},
		{name: "changed prior record", change: func(row *filler.ClipPipeline) { row.Stages[0].Note = "changed" }},
		{name: "already terminal", change: func(row *filler.ClipPipeline) { row.Disposition = filler.DispositionReady }},
		{name: "different stage start second", change: func(row *filler.ClipPipeline) {
			row.StageStartedAt = row.StageStartedAt.Add(time.Second)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drifted := current
			drifted.Stages = append([]filler.StageRecord(nil), current.Stages...)
			tt.change(&drifted)
			if err := commit.ValidateAgainst(drifted); !errors.Is(err, filler.ErrReadyStale) {
				t.Fatalf("ValidateAgainst error = %v, want ErrReadyStale", err)
			}
		})
	}
}
