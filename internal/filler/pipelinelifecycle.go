package filler

import "time"

// LifecycleState says who owns a clip now and whether the pipeline can make progress. It is the
// one classification consumed by the Incoming read model and run telemetry; callers must not
// reconstruct it from Status and Disposition independently.
type LifecycleState string

const (
	LifecycleRunnable      LifecycleState = "runnable"
	LifecycleInProgress    LifecycleState = "in_progress"
	LifecycleScheduled     LifecycleState = "scheduled"
	LifecycleNeedsDecision LifecycleState = "needs_decision"
	LifecycleAdmitted      LifecycleState = "admitted"
	LifecycleRejected      LifecycleState = "rejected"
	LifecycleDismissed     LifecycleState = "dismissed"
)

// PipelineFailureCode is a stable machine-readable class for execution failures. Content
// refusals retain their more specific RejectReason; only failure-shaped rejections are projected
// here so clients never have to parse RejectDetail or a stage note.
type PipelineFailureCode string

const (
	FailureStage       PipelineFailureCode = "stage_failed"
	FailureUnprobeable PipelineFailureCode = PipelineFailureCode(ReasonUnprobeable)
	FailureUnplayable  PipelineFailureCode = PipelineFailureCode(ReasonUnplayable)
)

// RecoveryAction distinguishes retrying failed WORK from overriding a content DECISION. They use
// different mutations because retry preserves the approval/grounding decision while restore
// explicitly overturns an eligible refusal.
type RecoveryAction string

const (
	RecoveryRetry   RecoveryAction = "retry"
	RecoveryRestore RecoveryAction = "restore"
)

// PipelineLifecycle is the small interface over the persisted state machine. RetryStage is set
// only with RecoveryRetry; the pipeline owns whether that retry requires internal force.
type PipelineLifecycle struct {
	State       LifecycleState
	FailureCode PipelineFailureCode
	Recovery    RecoveryAction
	RetryStage  StageID
}

// PipelineOverview is a bounded-cardinality snapshot of the whole durable conveyor. The fields
// are lifecycle states rather than storage dispositions so every consumer speaks in ownership and
// action, not table implementation.
type PipelineOverview struct {
	Runnable      int
	InProgress    int
	Scheduled     int
	NeedsDecision int
	Admitted      int
	Rejected      int
	Dismissed     int
}

// NoAdvanceReason explains a zero-advance pass from the same lifecycle vocabulary operators see.
// Empty means the pass advanced work and needs no special explanation.
func (o PipelineOverview) NoAdvanceReason(advanced int) string {
	if advanced > 0 {
		return ""
	}
	switch {
	case o.InProgress > 0:
		return "work_in_progress"
	case o.Runnable > 0:
		return "runnable_work_remains"
	case o.Scheduled > 0:
		return "waiting_for_retry"
	case o.NeedsDecision > 0 && o.Rejected > 0:
		return "waiting_for_decisions_and_recovery"
	case o.NeedsDecision > 0:
		return "waiting_for_decisions"
	case o.Rejected > 0:
		return "waiting_for_recovery"
	default:
		return "idle"
	}
}

// Add records `count` rows in one lifecycle state.
func (o *PipelineOverview) Add(state LifecycleState, count int) {
	switch state {
	case LifecycleRunnable:
		o.Runnable += count
	case LifecycleInProgress:
		o.InProgress += count
	case LifecycleScheduled:
		o.Scheduled += count
	case LifecycleNeedsDecision:
		o.NeedsDecision += count
	case LifecycleAdmitted:
		o.Admitted += count
	case LifecycleRejected:
		o.Rejected += count
	case LifecycleDismissed:
		o.Dismissed += count
	}
}

// SummarizePipelines is the in-memory adapter for tests and small stores. Production SQL groups
// equivalent facts before calling Add, but both paths cross Lifecycle for the actual decision.
func SummarizePipelines(rows []ClipPipeline, at time.Time) PipelineOverview {
	var out PipelineOverview
	for _, row := range rows {
		out.Add(row.Lifecycle(at).State, 1)
	}
	return out
}

// Lifecycle projects one persisted row into operator ownership and recovery semantics at `at`.
// Disposition wins over stage status: a review or rejection is never described as machine work
// merely because an older status survived on the current rung.
func (p ClipPipeline) Lifecycle(at time.Time) PipelineLifecycle {
	var out PipelineLifecycle
	switch p.Disposition {
	case DispositionReview:
		out.State = LifecycleNeedsDecision
	case DispositionFiled:
		out.State = LifecycleAdmitted
	case DispositionRejected:
		out.State = LifecycleRejected
	case DispositionDismissed:
		out.State = LifecycleDismissed
	default:
		switch {
		case p.Status == StatusRunning:
			out.State = LifecycleInProgress
		case !p.NextRun.IsZero() && p.NextRun.After(at):
			out.State = LifecycleScheduled
		default:
			out.State = LifecycleRunnable
		}
	}

	if p.Status == StatusFailed {
		out.FailureCode = FailureStage
		if p.Disposition == DispositionRunning {
			out.Recovery, out.RetryStage = RecoveryRetry, p.Stage
		}
	}
	if p.Disposition == DispositionRejected {
		switch {
		case p.Status == StatusFailed && p.Stage == StageProbe && p.RejectReason == ReasonUnprobeable:
			out.FailureCode = FailureUnprobeable
			out.Recovery, out.RetryStage = RecoveryRetry, StageProbe
		case p.Status == StatusFailed && p.Stage == StageTranscode && p.RejectReason == ReasonUnplayable:
			out.FailureCode = FailureUnplayable
			out.Recovery, out.RetryStage = RecoveryRetry, StageTranscode
		case p.RejectReason.Soft():
			out.Recovery = RecoveryRestore
		}
	}
	return out
}
