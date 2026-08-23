package filler

import (
	"testing"
	"time"
)

func TestClipPipelineLifecycle(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	tests := []struct {
		name string
		row  ClipPipeline
		want PipelineLifecycle
	}{
		{
			name: "queued and due is runnable",
			row: ClipPipeline{Disposition: DispositionRunning, Stage: StageTag,
				Status: StatusQueued, NextRun: now.Add(-time.Minute)},
			want: PipelineLifecycle{State: LifecycleRunnable},
		},
		{
			name: "active stage is in progress",
			row: ClipPipeline{Disposition: DispositionRunning, Stage: StageTranscode,
				Status: StatusRunning},
			want: PipelineLifecycle{State: LifecycleInProgress},
		},
		{
			name: "failed stage in backoff is scheduled and retryable",
			row: ClipPipeline{Disposition: DispositionRunning, Stage: StageVision,
				Status: StatusFailed, Attempts: 2, NextRun: now.Add(time.Hour)},
			want: PipelineLifecycle{State: LifecycleScheduled, FailureCode: FailureStage,
				Recovery: RecoveryRetry, RetryStage: StageVision},
		},
		{
			name: "failed stage whose backoff expired is runnable and retryable",
			row: ClipPipeline{Disposition: DispositionRunning, Stage: StageVision,
				Status: StatusFailed, Attempts: 2, NextRun: now.Add(-time.Minute)},
			want: PipelineLifecycle{State: LifecycleRunnable, FailureCode: FailureStage,
				Recovery: RecoveryRetry, RetryStage: StageVision},
		},
		{
			name: "human handoff is a decision",
			row:  ClipPipeline{Disposition: DispositionReview, Stage: StageScore, Status: StatusDone},
			want: PipelineLifecycle{State: LifecycleNeedsDecision},
		},
		{
			name: "filed is admitted",
			row:  ClipPipeline{Disposition: DispositionFiled, Stage: StageScore, Status: StatusDone},
			want: PipelineLifecycle{State: LifecycleAdmitted},
		},
		{
			name: "exhausted transcode execution can retry after repair",
			row: ClipPipeline{Disposition: DispositionRejected, Stage: StageTranscode,
				Status: StatusFailed, Attempts: MaxAttempts, RejectReason: ReasonUnplayable},
			want: PipelineLifecycle{State: LifecycleRejected, FailureCode: FailureUnplayable,
				Recovery: RecoveryRetry, RetryStage: StageTranscode},
		},
		{
			name: "language refusal is an overridable content decision",
			row: ClipPipeline{Disposition: DispositionRejected, Stage: StageLanguage,
				Status: StatusDone, RejectReason: ReasonLanguage},
			want: PipelineLifecycle{State: LifecycleRejected, Recovery: RecoveryRestore},
		},
		{
			name: "measured dead air is not recoverable",
			row: ClipPipeline{Disposition: DispositionRejected, Stage: StageTranscode,
				Status: StatusDone, RejectReason: ReasonSilentContent},
			want: PipelineLifecycle{State: LifecycleRejected},
		},
		{
			name: "operator dismissal remains distinct",
			row:  ClipPipeline{Disposition: DispositionDismissed, Stage: StageScore, Status: StatusDone},
			want: PipelineLifecycle{State: LifecycleDismissed},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.row.Lifecycle(now); got != tc.want {
				t.Fatalf("Lifecycle() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPipelineOverviewNoAdvanceReason(t *testing.T) {
	tests := []struct {
		name     string
		overview PipelineOverview
		advanced int
		want     string
	}{
		{name: "advanced", overview: PipelineOverview{Runnable: 2}, advanced: 1, want: ""},
		{name: "retry clock", overview: PipelineOverview{Scheduled: 2}, want: "waiting_for_retry"},
		{name: "human", overview: PipelineOverview{NeedsDecision: 3}, want: "waiting_for_decisions"},
		{name: "failed", overview: PipelineOverview{Rejected: 4}, want: "waiting_for_recovery"},
		{name: "mixed", overview: PipelineOverview{NeedsDecision: 3, Rejected: 4}, want: "waiting_for_decisions_and_recovery"},
		{name: "empty", overview: PipelineOverview{}, want: "idle"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.overview.NoAdvanceReason(tc.advanced); got != tc.want {
				t.Fatalf("NoAdvanceReason() = %q, want %q", got, tc.want)
			}
		})
	}
}
