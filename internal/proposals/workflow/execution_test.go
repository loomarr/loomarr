package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/proposals"
)

func TestWorkflowClaimReturnsVersionedAttemptTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 22, 14, 0, 0, 0, time.UTC)
	repo := &executionRepositorySpy{works: []Work{{
		Version: WorkflowVersion1, JobID: "job-1", Attempt: 2,
		Intent: proposals.Intent{Description: "Saturday morning cartoons"},
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

	_, err := workflow.Complete(context.Background(), work, proposals.Proposal{})
	if !errors.Is(err, proposals.ErrNoGroundedTitles) {
		t.Fatalf("Complete empty Proposal error = %v, want ErrNoGroundedTitles", err)
	}
	if repo.completed != nil {
		t.Fatal("empty Proposal reached repository")
	}

	prop := proposals.Proposal{Lineup: []proposals.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", InLibrary: true,
	}}, Trace: selectedDecisionTrace("movie:tmdb:90000603", "Marigold Protocol", "library")}
	_, err = workflow.Complete(context.Background(), work, prop)
	if err != nil {
		t.Fatalf("Complete grounded Proposal: %v", err)
	}
	if repo.completed == nil || repo.completed.Work.JobID != work.JobID ||
		repo.completed.Work.Attempt != work.Attempt || len(repo.completed.Proposal.Lineup) != 1 {
		t.Fatalf("completion = %+v", repo.completed)
	}

	repo.completed = nil
	prop.Trace = proposals.DecisionTrace{}
	if _, err := workflow.Complete(context.Background(), work, prop); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Complete absent trace error = %v, want ErrInvalidState", err)
	}
	if repo.completed != nil {
		t.Fatal("absent trace reached repository")
	}

	repo.completed = nil
	prop.Trace = proposals.DecisionTrace{Version: proposals.DecisionTraceVersion + 1}
	if _, err := workflow.Complete(context.Background(), work, prop); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Complete invalid trace error = %v, want ErrInvalidState", err)
	}
	if repo.completed != nil {
		t.Fatal("invalid trace reached repository")
	}

	repo.completed = nil
	refusedOnly := proposals.Proposal{Refused: []proposals.RefusedPick{{Item: proposals.ProposalItem{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", InLibrary: true,
	}, Reason: proposals.ReasonOverCeiling}}, Trace: proposals.DecisionTrace{
		Version: proposals.DecisionTraceVersion, SurfacedTotal: 1, RecordedTotal: 1,
		Candidates: []proposals.DecisionCandidate{{Key: "movie:tmdb:90000603", Name: "Marigold Protocol", Ownership: "library",
			Rank: proposals.RankTuple{TieKey: "movie:tmdb:90000603"}, Disposition: proposals.DispositionRefused, Reason: proposals.ReasonOverCeiling}},
	}}
	if _, err := workflow.Complete(context.Background(), work, refusedOnly); err != nil {
		t.Fatalf("Complete refused-only review Proposal: %v", err)
	}
	if repo.completed == nil || len(repo.completed.Proposal.Refused) != 1 {
		t.Fatalf("refused-only review completion = %+v", repo.completed)
	}
}

func TestWorkflowCompletePreservesStaleAttemptRejection(t *testing.T) {
	t.Parallel()

	repo := &executionRepositorySpy{completeErr: ErrStaleAttempt}
	workflow := newWorkflow(repo)
	_, err := workflow.Complete(context.Background(), Work{
		Version: WorkflowVersion1, JobID: "job-1", Attempt: 1,
	}, proposals.Proposal{Lineup: []proposals.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol",
	}}, Trace: selectedDecisionTrace("movie:tmdb:90000603", "Marigold Protocol", "acquisition")})
	if !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("Complete stale Attempt error = %v, want ErrStaleAttempt", err)
	}
}

func selectedDecisionTrace(key, name, ownership string) proposals.DecisionTrace {
	return proposals.DecisionTrace{
		Version: proposals.DecisionTraceVersion, SurfacedTotal: 1, RecordedTotal: 1,
		Candidates: []proposals.DecisionCandidate{{Key: key, Name: name, Ownership: ownership,
			Rank: proposals.RankTuple{TieKey: key}, Disposition: proposals.DispositionSelected, Reason: "selected"}},
	}
}

func TestWorkflowFailBoundsCodeButPreservesPrivateDiagnostic(t *testing.T) {
	t.Parallel()

	repo := &executionRepositorySpy{}
	workflow := newWorkflow(repo)
	work := Work{Version: WorkflowVersion1, JobID: "job-1", Attempt: 2}
	diagnostic := "provider returned private diagnostic"

	if err := workflow.Fail(context.Background(), work, "future_code", diagnostic); err != nil {
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

func (r *executionRepositorySpy) CompleteAttempt(_ context.Context, completion Completion) (CommittedProposal, error) {
	r.completed = &completion
	return CommittedProposal{}, r.completeErr
}

func (r *executionRepositorySpy) FailAttempt(_ context.Context, failure AttemptFailure) error {
	r.failed = &failure
	return r.failErr
}
