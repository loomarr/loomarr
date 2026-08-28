package proposalworkflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/suggest"
)

var ErrStaleAttempt = errors.New("proposal workflow: stale attempt")

// Work is a versioned lease token. The Job id alone is never sufficient to
// commit an activity result because an expired worker may finish after recovery
// has already issued the next Attempt.
type Work = suggest.WorkflowWork

// Completion is committed atomically: the Proposal and the terminal Job and
// Attempt transitions either all become visible or none do.
type Completion struct {
	Work     Work
	Proposal suggest.Proposal
}

type AttemptFailure struct {
	Work       Work
	Code       FailureCode
	Diagnostic string
	TraceJSON  string
}

type executionRepository interface {
	ClaimAttempts(context.Context, time.Time, time.Duration, int) ([]Work, error)
	CompleteAttempt(context.Context, Completion) (suggest.WorkflowProposal, error)
	FailAttempt(context.Context, AttemptFailure) error
}

// Claim leases bounded work. Lease recovery and monotonically numbered Attempt
// creation are one repository transaction behind this module-owned command.
func (w *Workflow) Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Work, error) {
	if lease <= 0 || limit <= 0 {
		return nil, fmt.Errorf("%w: invalid claim lease=%s limit=%d", ErrInvalidState, lease, limit)
	}
	if w.execution == nil {
		return nil, fmt.Errorf("%w: execution repository unavailable", ErrInvalidState)
	}
	works, err := w.execution.ClaimAttempts(ctx, now, lease, limit)
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
func (w *Workflow) Complete(ctx context.Context, work Work, proposal suggest.Proposal) (suggest.WorkflowProposal, error) {
	if err := validateWork(work); err != nil {
		return suggest.WorkflowProposal{}, err
	}
	if len(proposal.Lineup)+len(proposal.Acquisitions) == 0 {
		return suggest.WorkflowProposal{}, suggest.ErrNoGroundedTitles
	}
	if err := validateProposalIdentities(proposal); err != nil {
		return suggest.WorkflowProposal{}, err
	}
	if w.execution == nil {
		return suggest.WorkflowProposal{}, fmt.Errorf("%w: execution repository unavailable", ErrInvalidState)
	}
	completed, err := w.execution.CompleteAttempt(ctx, Completion{Work: work, Proposal: proposal})
	if err != nil {
		return suggest.WorkflowProposal{}, fmt.Errorf("complete Proposal Job %q Attempt %d: %w", work.JobID, work.Attempt, err)
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
	if boundedCode != FailureNoGroundedTitles {
		boundedCode = FailureGenerationFailed
	}
	if w.execution == nil {
		return fmt.Errorf("%w: execution repository unavailable", ErrInvalidState)
	}
	trace := ""
	if len(traceJSON) > 0 {
		trace = traceJSON[0]
	}
	if err := w.execution.FailAttempt(ctx, AttemptFailure{
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

func validateProposalIdentities(proposal suggest.Proposal) error {
	groups := [][]suggest.ProposalItem{proposal.Lineup, proposal.Acquisitions, proposal.Alternates}
	for _, items := range groups {
		for _, item := range items {
			if _, err := item.Key(); err != nil {
				return fmt.Errorf("%w: Proposal contains an ungrounded identity: %v", ErrInvalidState, err)
			}
		}
	}
	return nil
}
