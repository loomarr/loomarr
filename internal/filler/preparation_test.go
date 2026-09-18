package filler

import (
	"testing"
	"time"
)

func TestProjectPreparationProgress(t *testing.T) {
	tests := []struct {
		name string
		row  ClipPipeline
		want int
	}{
		{name: "legacy is unknown", row: ClipPipeline{}, want: -1},
		{name: "enrolled starts at zero", row: ClipPipeline{PreparationAttempt: 1, PreparationProgress: 0}, want: 0},
		{name: "measured current stage contributes fraction", row: ClipPipeline{
			PreparationAttempt: 1, Disposition: DispositionRunning, Stage: StageTranscode,
			Status: StatusRunning, Progress: 50,
			Stages: []StageRecord{{Stage: StageProbe, Status: StatusDone}},
		}, want: 16},
		{name: "skips count once and duplicates do not", row: ClipPipeline{
			PreparationAttempt: 1,
			Stages:             []StageRecord{{Stage: StageProbe, Status: StatusDone}, {Stage: StageProbe, Status: StatusDone}, {Stage: StageTranscode, Status: StatusSkipped}},
		}, want: 22},
		{name: "persisted maximum prevents regression", row: ClipPipeline{
			PreparationAttempt: 1, PreparationProgress: 61,
			Stages: []StageRecord{{Stage: StageProbe, Status: StatusDone}},
		}, want: 61},
		{name: "ready is exact", row: ClipPipeline{PreparationAttempt: 1, PreparationProgress: 72, Disposition: DispositionReady}, want: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectPreparationProgress(tt.row); got != tt.want {
				t.Fatalf("ProjectPreparationProgress() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEstimatePreparationReady(t *testing.T) {
	now := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	current := PreparationWork{DurationMs: 30_000, Pipeline: ClipPipeline{
		Stage: StageTranscode, Status: StatusRunning, Progress: 50, Disposition: DispositionRunning,
		PreparationAttempt: 1, PreparationProgress: 16, PreparationStartedAt: now.Add(-time.Minute), UpdatedAt: now,
		Stages: []StageRecord{{Stage: StageProbe, Status: StatusDone}},
	}}
	history := []PreparationWork{
		readyPreparationSample(now.Add(-time.Hour), 30_000, 10*time.Second),
		readyPreparationSample(now.Add(-2*time.Hour), 25_000, 20*time.Second),
		readyPreparationSample(now.Add(-3*time.Hour), 29_000, 30*time.Second),
	}
	estimate, ok := EstimatePreparationReady(current, history, nil, now)
	if !ok || estimate.Lower != 75*time.Second || estimate.Upper != 225*time.Second {
		t.Fatalf("estimate = %+v, ok=%t; want 75s–225s", estimate, ok)
	}

	withQueue, ok := EstimatePreparationReady(current, history, []PreparationWork{{
		DurationMs: 30_000,
		Pipeline: ClipPipeline{Stage: StageProbe, Status: StatusQueued, Disposition: DispositionRunning,
			PreparationAttempt: 1, PreparationProgress: 0, PreparationStartedAt: now, UpdatedAt: now},
	}}, now)
	if !ok || withQueue.Lower != 165*time.Second || withQueue.Upper != 495*time.Second {
		t.Fatalf("queue estimate = %+v, ok=%t; want 165s–495s", withQueue, ok)
	}

	for _, tc := range []struct {
		name string
		edit func(*PreparationWork, *[]PreparationWork)
	}{
		{name: "insufficient history", edit: func(_ *PreparationWork, samples *[]PreparationWork) { *samples = (*samples)[:2] }},
		{name: "scheduled retry", edit: func(work *PreparationWork, _ *[]PreparationWork) {
			work.Pipeline.Status = StatusFailed
			work.Pipeline.NextRun = now.Add(time.Minute)
		}},
		{name: "restart reset", edit: func(work *PreparationWork, _ *[]PreparationWork) {
			work.Pipeline.PreparationStartReason = PreparationStartedByRestart
			work.Pipeline.PreparationProgress = 0
		}},
		{name: "stale", edit: func(work *PreparationWork, _ *[]PreparationWork) {
			work.Pipeline.UpdatedAt = now.Add(-PreparationStaleAfter - time.Second)
		}},
		{name: "different duration bucket", edit: func(work *PreparationWork, _ *[]PreparationWork) { work.DurationMs = 120_000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := current
			samples := append([]PreparationWork(nil), history...)
			tc.edit(&work, &samples)
			if _, found := EstimatePreparationReady(work, samples, nil, now); found {
				t.Fatal("estimate was emitted without trustworthy evidence")
			}
		})
	}
}

func readyPreparationSample(finishedAt time.Time, durationMs int64, perStage time.Duration) PreparationWork {
	startedAt := finishedAt.Add(-perStage * time.Duration(len(StageOrder)))
	row := ClipPipeline{
		Stage: StageScore, Status: StatusDone, Disposition: DispositionReady,
		PreparationAttempt: 1, PreparationProgress: 100, PreparationStartedAt: startedAt, UpdatedAt: finishedAt,
	}
	for _, stage := range StageOrder {
		row.Stages = append(row.Stages, StageRecord{Stage: stage, Status: StatusDone, Attempts: 1, StartedAt: startedAt, At: startedAt.Add(perStage)})
		startedAt = startedAt.Add(perStage)
	}
	return PreparationWork{Pipeline: row, DurationMs: durationMs}
}
