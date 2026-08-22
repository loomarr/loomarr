package proposalworkflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/suggest"
)

func TestWorkflowClaimReturnsVersionedAttemptTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 22, 14, 0, 0, 0, time.UTC)
	repo := &executionRepositorySpy{works: []Work{{
		Version: WorkflowVersion1, JobID: "job-1", Attempt: 2,
		Intent: suggest.Intent{Description: "Saturday morning cartoons"},
	}}}
	workflow := newWorkflow(repo)

	works, err := workflow.Claim(context.Background(), now, 2*time.Minute, 3)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if repo.claimedAt != now || repo.lease != 2*time.Minute || repo.limit != 3 {
		t.Fatalf("claim request = (%v, %v, %d)", repo.claimedAt, repo.lease, repo.limit)
	}
	if len(works) != 1 || works[0].JobID != "job-1" || works[0].Attempt != 2 {
		t.Fatalf("works = %+v", works)
	}
}

func TestWorkflowClaimRejectsUnknownPersistedWorkVersion(t *testing.T) {
	t.Parallel()

	repo := &executionRepositorySpy{works: []Work{{
		Version: WorkflowVersion1 + 1, JobID: "job-future", Attempt: 1,
	}}}
	workflow := newWorkflow(repo)

	_, err := workflow.Claim(context.Background(), time.Now(), time.Minute, 1)
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Claim future work error = %v, want ErrInvalidState", err)
	}
}

func TestWorkflowCompleteUsesCurrentAttemptTokenAndRejectsEmptyResult(t *testing.T) {
	t.Parallel()

	work := Work{Version: WorkflowVersion1, JobID: "job-1", Attempt: 2}
	repo := &executionRepositorySpy{}
	workflow := newWorkflow(repo)

	err := workflow.Complete(context.Background(), work, suggest.Proposal{})
	if !errors.Is(err, suggest.ErrNoGroundedTitles) {
		t.Fatalf("Complete empty Proposal error = %v, want ErrNoGroundedTitles", err)
	}
	if repo.completed != nil {
		t.Fatal("empty Proposal reached repository")
	}

	proposal := suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", InLibrary: true,
	}}}
	err = workflow.Complete(context.Background(), work, proposal)
	if err != nil {
		t.Fatalf("Complete grounded Proposal: %v", err)
	}
	if repo.completed == nil || repo.completed.Work.JobID != work.JobID ||
		repo.completed.Work.Attempt != work.Attempt || len(repo.completed.Proposal.Lineup) != 1 {
		t.Fatalf("completion = %+v", repo.completed)
	}
}

func TestWorkflowCompletePreservesStaleAttemptRejection(t *testing.T) {
	t.Parallel()

	repo := &executionRepositorySpy{completeErr: ErrStaleAttempt}
	workflow := newWorkflow(repo)
	err := workflow.Complete(context.Background(), Work{
		Version: WorkflowVersion1, JobID: "job-1", Attempt: 1,
	}, suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}})
	if !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("Complete stale Attempt error = %v, want ErrStaleAttempt", err)
	}
}

func TestWorkflowFailBoundsCodeButPreservesPrivateDiagnostic(t *testing.T) {
	t.Parallel()

	repo := &executionRepositorySpy{}
	workflow := newWorkflow(repo)
	work := Work{Version: WorkflowVersion1, JobID: "job-1", Attempt: 2}
	diagnostic := "provider returned private diagnostic"

	if err := workflow.Fail(context.Background(), work, FailureCode("future_code"), diagnostic); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if repo.failed == nil || repo.failed.Code != FailureGenerationFailed || repo.failed.Diagnostic != diagnostic {
		t.Fatalf("failure command = %+v", repo.failed)
	}
}

type executionRepositorySpy struct {
	recordingRepository
	works       []Work
	claimErr    error
	claimedAt   time.Time
	lease       time.Duration
	limit       int
	completed   *Completion
	completeErr error
	failed      *AttemptFailure
	failErr     error
}

func (r *executionRepositorySpy) ClaimAttempts(_ context.Context, now time.Time, lease time.Duration, limit int) ([]Work, error) {
	r.claimedAt, r.lease, r.limit = now, lease, limit
	return append([]Work(nil), r.works...), r.claimErr
}

func (r *executionRepositorySpy) CompleteAttempt(_ context.Context, completion Completion) error {
	r.completed = &completion
	return r.completeErr
}

func (r *executionRepositorySpy) FailAttempt(_ context.Context, failure AttemptFailure) error {
	r.failed = &failure
	return r.failErr
}
