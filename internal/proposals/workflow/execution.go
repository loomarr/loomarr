package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/proposals"
)

// ErrStaleAttempt reports a terminal write from an Attempt that recovery or a requeue replaced: its
// compare-and-swap lost because a newer Attempt owns the Job. The worker drops that result, since
// the replacement execution, not this one, decides the Job's outcome.
var ErrStaleAttempt = errors.New("proposal workflow: stale attempt")

// Work is a versioned lease token and the activity input the worker executes. The Job id alone is
// never sufficient to commit an activity result because an expired worker may finish after recovery
// has already issued the next Attempt; Version plus Attempt form the compare-and-swap token for
// terminal writes.
type Work struct {
	Version   int
	JobID     string
	Attempt   int
	Kind      string
	CreatedBy string
	// ChannelID names the channel a refine or re-curation job re-curates; empty for a request.
	ChannelID string
	Intent    proposals.Intent
}

// CommittedProposal is the durably committed Proposal returned to the caller once generation (or a
// recording) succeeds, for the optional approval grants.
type CommittedProposal struct {
	ID        string
	JobID     string
	CreatedBy string
	Proposal  proposals.Proposal
	CreatedAt time.Time
}

// Submission is the result of submitting an Intent: the new Job, or the cached Proposal a recent
// successful Job of the same Intent already produced.
type Submission struct {
	JobID          string
	CachedProposal *CommittedProposal
}

// Completion is committed atomically: the Proposal and the terminal Job and
// Attempt transitions either all become visible or none do.
type Completion struct {
	Work     Work
	Proposal proposals.Proposal
}

type AttemptFailure struct {
	Work       Work
	Code       FailureCode
	Diagnostic string
	TraceJSON  string
}

// Claim leases bounded work. Lease recovery and monotonically numbered Attempt
// creation are one repository transaction behind this module-owned command.
func (w *Workflow) Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Work, error) {
	if lease <= 0 || limit <= 0 {
		return nil, fmt.Errorf("%w: invalid claim lease=%s limit=%d", ErrInvalidState, lease, limit)
	}
	works, err := w.repository.ClaimAttempts(ctx, now, lease, limit)
	if err != nil {
		return nil, fmt.Errorf("claim Proposal Jobs: %w", err)
	}
	for _, work := range works {
		if err := validateWork(work); err != nil {
			return nil, err
		}
	}
	return append([]Work(nil), works...), nil
}

// Complete atomically publishes a grounded Proposal and closes the current
// Attempt. A stale lease token must be rejected by the repository guard.
func (w *Workflow) Complete(ctx context.Context, work Work, prop proposals.Proposal) (CommittedProposal, error) {
	if err := validateWork(work); err != nil {
		return CommittedProposal{}, err
	}
	if len(prop.Lineup)+len(prop.Acquisitions)+len(prop.Alternates)+len(prop.Refused) == 0 {
		return CommittedProposal{}, proposals.ErrNoGroundedTitles
	}
	if err := validateProposalIdentities(prop); err != nil {
		return CommittedProposal{}, err
	}
	if prop.Trace.Version != proposals.DecisionTraceVersion {
		return CommittedProposal{}, fmt.Errorf("%w: Proposal decision trace version = %d, want %d", ErrInvalidState, prop.Trace.Version, proposals.DecisionTraceVersion)
	}
	if err := proposals.ValidateDecisionTrace(prop.Trace); err != nil {
		return CommittedProposal{}, fmt.Errorf("%w: invalid Proposal decision trace: %v", ErrInvalidState, err)
	}
	completed, err := w.repository.CompleteAttempt(ctx, Completion{Work: work, Proposal: prop})
	if err != nil {
		return CommittedProposal{}, fmt.Errorf("complete Proposal Job %q Attempt %d: %w", work.JobID, work.Attempt, err)
	}
	return completed, nil
}

// Fail closes the current Attempt while preserving the private diagnostic only
// for operator logging. Unknown codes collapse to the bounded general failure.
func (w *Workflow) Fail(ctx context.Context, work Work, code string, diagnostic string, traceJSON ...string) error {
	if err := validateWork(work); err != nil {
		return err
	}
	boundedCode := FailureCode(code)
	if boundedCode != FailureNoGroundedTitles && boundedCode != FailureSelectionEmpty && boundedCode != FailureBudgetExhausted {
		boundedCode = FailureGenerationFailed
	}
	trace := ""
	if len(traceJSON) > 0 {
		trace = traceJSON[0]
	}
	if err := w.repository.FailAttempt(ctx, AttemptFailure{
		Work: work, Code: boundedCode, Diagnostic: diagnostic,
		TraceJSON: trace,
	}); err != nil {
		return fmt.Errorf("fail Proposal Job %q Attempt %d: %w", work.JobID, work.Attempt, err)
	}
	return nil
}

func validateWork(work Work) error {
	if work.Version != WorkflowVersion1 || work.JobID == "" || work.Attempt <= 0 {
		return fmt.Errorf("%w: work version=%d job=%q Attempt=%d", ErrInvalidState, work.Version, work.JobID, work.Attempt)
	}
	return nil
}

func validateProposalIdentities(prop proposals.Proposal) error {
	groups := [][]proposals.ProposalItem{prop.Lineup, prop.Acquisitions, prop.Alternates}
	for _, items := range groups {
		for _, item := range items {
			if _, err := item.Key(); err != nil {
				return fmt.Errorf("%w: Proposal contains an ungrounded identity: %v", ErrInvalidState, err)
			}
		}
	}
	for _, refused := range prop.Refused {
		if _, err := refused.Item.Key(); err != nil {
			return fmt.Errorf("%w: Proposal contains an ungrounded refused identity: %v", ErrInvalidState, err)
		}
	}
	return nil
}
