package proposalworkflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestWorkflowInspectGeneratingIsAuthoritativeAndCallerScoped(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	repo := &recordingRepository{record: Record{
		Version: WorkflowVersion1,
		JobID:   "job-1",
		OwnerID: "member-1",
		Status:  JobRunning,
		Intent:  suggest.Intent{Description: "Saturday morning cartoons"},
		Attempts: []Attempt{{
			Version: WorkflowVersion1, Number: 1, Status: AttemptRunning, StartedAt: started,
		}},
		CreatedAt: started.Add(-time.Second),
		UpdatedAt: started,
	}}
	workflow := newWorkflow(repo)

	journey, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-1")
	if err != nil {
		t.Fatalf("Inspect own Proposal Job: %v", err)
	}
	if repo.loadedID != "job-1" {
		t.Fatalf("repository loaded %q, want job-1", repo.loadedID)
	}
	if journey.Version != WorkflowVersion1 || journey.JobID != "job-1" {
		t.Fatalf("journey identity = version %d job %q", journey.Version, journey.JobID)
	}
	if journey.Milestone != MilestoneGenerating {
		t.Fatalf("milestone = %q, want %q", journey.Milestone, MilestoneGenerating)
	}
	if journey.Intent.Description != "Saturday morning cartoons" {
		t.Fatalf("intent = %+v", journey.Intent)
	}
	if len(journey.Attempts) != 1 || journey.Attempts[0].Status != AttemptRunning {
		t.Fatalf("attempt history = %+v", journey.Attempts)
	}
	if len(journey.Actions) != 1 || journey.Actions[0] != ActionWait {
		t.Fatalf("actions = %v, want [%s]", journey.Actions, ActionWait)
	}

	_, err = workflow.Inspect(context.Background(), Viewer{UserID: "member-2"}, "job-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Inspect another member's Proposal Job error = %v, want ErrForbidden", err)
	}
}

func TestJourneyTraceClonesDoNotAliasRepositoryEvidence(t *testing.T) {
	t.Parallel()

	trace := suggest.DecisionTrace{Version: suggest.DecisionTraceVersion, SurfacedTotal: 1, RecordedTotal: 1,
		Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:603"}}}
	proposal := &ProposalRef{Proposal: suggest.Proposal{Trace: trace}}
	failure := &Failure{Trace: trace}
	proposalClone := cloneProposal(proposal)
	failureClone := cloneFailure(failure)
	proposalClone.Proposal.Trace.Candidates[0].Key = "mutated-proposal"
	failureClone.Trace.Candidates[0].Key = "mutated-failure"
	if proposal.Proposal.Trace.Candidates[0].Key != "movie:tmdb:603" || failure.Trace.Candidates[0].Key != "movie:tmdb:603" {
		t.Fatalf("caller mutation changed repository evidence: proposal=%+v failure=%+v", proposal, failure)
	}
}

func TestWorkflowInspectAwaitingApprovalDerivesRoleSafeActions(t *testing.T) {
	t.Parallel()

	repo := &recordingRepository{record: Record{
		Version: WorkflowVersion1,
		JobID:   "job-review",
		OwnerID: "member-1",
		Status:  JobDone,
		Intent:  suggest.Intent{Description: "Cozy mysteries"},
		Attempts: []Attempt{{
			Version: WorkflowVersion1, Number: 1, Status: AttemptSucceeded,
		}},
		Proposal: &ProposalRef{ID: "proposal-1", Status: ProposalSubmitted},
	}}
	workflow := newWorkflow(repo)

	member, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-review")
	if err != nil {
		t.Fatalf("Inspect as owner: %v", err)
	}
	if member.Milestone != MilestoneAwaitingApproval {
		t.Fatalf("member milestone = %q", member.Milestone)
	}
	if len(member.Actions) != 1 || member.Actions[0] != ActionWait {
		t.Fatalf("member actions = %v, want [%s]", member.Actions, ActionWait)
	}
	if member.Proposal == nil || member.Proposal.ID != "proposal-1" {
		t.Fatalf("member proposal = %+v", member.Proposal)
	}

	admin, err := workflow.Inspect(context.Background(), Viewer{Admin: true}, "job-review")
	if err != nil {
		t.Fatalf("Inspect as admin: %v", err)
	}
	if len(admin.Actions) != 1 || admin.Actions[0] != ActionReview {
		t.Fatalf("admin actions = %v, want [%s]", admin.Actions, ActionReview)
	}
}

func TestWorkflowInspectDeniedPreservesRecoveryActions(t *testing.T) {
	t.Parallel()

	workflow := newWorkflow(&recordingRepository{record: Record{
		Version: WorkflowVersion1,
		JobID:   "job-denied",
		OwnerID: "member-1",
		Status:  JobDone,
		Intent:  suggest.Intent{Description: "Late-night creature features"},
		Attempts: []Attempt{{
			Version: WorkflowVersion1, Number: 1, Status: AttemptSucceeded,
		}},
		Proposal: &ProposalRef{ID: "proposal-denied", Status: ProposalDenied},
	}})

	journey, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-denied")
	if err != nil {
		t.Fatalf("Inspect denied Journey: %v", err)
	}
	if journey.Milestone != MilestoneDenied {
		t.Fatalf("milestone = %q, want %q", journey.Milestone, MilestoneDenied)
	}
	wantActions := []Action{ActionEdit, ActionRetry}
	if len(journey.Actions) != len(wantActions) {
		t.Fatalf("actions = %v, want %v", journey.Actions, wantActions)
	}
	for i := range wantActions {
		if journey.Actions[i] != wantActions[i] {
			t.Fatalf("actions = %v, want %v", journey.Actions, wantActions)
		}
	}
}

func TestWorkflowInspectApprovedTracksFirstLiveSeparatelyFromCurrentChannelStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		channel     ChannelRef
		reachedLive bool
		milestone   Milestone
	}{
		{
			name: "building toward first live", channel: ChannelRef{ID: "channel-1", Status: schedule.StatusBuilding},
			milestone: MilestoneBuilding,
		},
		{
			name: "currently live", channel: ChannelRef{ID: "channel-1", Status: schedule.StatusLive},
			milestone: MilestoneLive,
		},
		{
			name: "paused after first live", channel: ChannelRef{ID: "channel-1", Status: schedule.StatusPaused},
			reachedLive: true, milestone: MilestoneLive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workflow := newWorkflow(&recordingRepository{record: Record{
				Version: WorkflowVersion1,
				JobID:   "job-approved",
				OwnerID: "member-1",
				Status:  JobDone,
				Attempts: []Attempt{{
					Version: WorkflowVersion1, Number: 1, Status: AttemptSucceeded,
				}},
				Proposal: &ProposalRef{
					ID: "proposal-approved", Status: ProposalApproved,
				},
				Channel:     &tt.channel,
				ReachedLive: tt.reachedLive,
			}})

			journey, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-approved")
			if err != nil {
				t.Fatalf("Inspect approved Journey: %v", err)
			}
			if journey.Milestone != tt.milestone {
				t.Fatalf("milestone = %q, want %q", journey.Milestone, tt.milestone)
			}
			if journey.Channel == nil || journey.Channel.Status != tt.channel.Status {
				t.Fatalf("channel = %+v, want status %q", journey.Channel, tt.channel.Status)
			}
			if len(journey.Actions) != 1 || journey.Actions[0] != ActionOpenChannel {
				t.Fatalf("actions = %v, want [%s]", journey.Actions, ActionOpenChannel)
			}
		})
	}
}

func TestWorkflowInspectApprovedWithoutChannelFailsClosed(t *testing.T) {
	t.Parallel()

	workflow := newWorkflow(&recordingRepository{record: Record{
		Version:  WorkflowVersion1,
		JobID:    "job-corrupt",
		OwnerID:  "member-1",
		Status:   JobDone,
		Attempts: []Attempt{{Version: WorkflowVersion1, Number: 1, Status: AttemptSucceeded}},
		Proposal: &ProposalRef{ID: "proposal-approved", Status: ProposalApproved},
	}})

	_, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-corrupt")
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Inspect impossible approved Journey error = %v, want ErrInvalidState", err)
	}
}

func TestWorkflowInspectRejectsStructurallyImpossibleAttemptHistory(t *testing.T) {
	t.Parallel()

	tests := []Record{
		{
			Version: WorkflowVersion1, JobID: "running-without-attempt", OwnerID: "member-1",
			Status: JobRunning,
		},
		{
			Version: WorkflowVersion1, JobID: "done-with-running-attempt", OwnerID: "member-1",
			Status: JobDone, Attempts: []Attempt{{Version: WorkflowVersion1, Number: 1, Status: AttemptRunning}},
			Proposal: &ProposalRef{ID: "proposal-1", Status: ProposalSubmitted},
		},
		{
			Version: WorkflowVersion1, JobID: "failed-with-succeeded-attempt", OwnerID: "member-1",
			Status: JobFailed, Attempts: []Attempt{{Version: WorkflowVersion1, Number: 1, Status: AttemptSucceeded}},
			FailureCode: FailureGenerationFailed,
		},
		{
			Version: WorkflowVersion1, JobID: "non-monotonic-attempts", OwnerID: "member-1",
			Status: JobRunning, Attempts: []Attempt{
				{Version: WorkflowVersion1, Number: 1, Status: AttemptFailed},
				{Version: WorkflowVersion1, Number: 3, Status: AttemptRunning},
			},
		},
		{
			Version: WorkflowVersion1, JobID: "future-attempt-version", OwnerID: "member-1",
			Status: JobRunning, Attempts: []Attempt{{
				Version: WorkflowVersion1 + 1, Number: 1, Status: AttemptRunning,
			}},
		},
	}

	for _, record := range tests {
		record := record
		t.Run(record.JobID, func(t *testing.T) {
			t.Parallel()
			workflow := newWorkflow(&recordingRepository{record: record})
			_, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, record.JobID)
			if !errors.Is(err, ErrInvalidState) {
				t.Fatalf("Inspect corrupt Journey error = %v, want ErrInvalidState", err)
			}
		})
	}
}

func TestWorkflowInspectProjectsLegacyJobWithoutInventingAttemptHistory(t *testing.T) {
	t.Parallel()

	workflow := newWorkflow(&recordingRepository{record: Record{
		Version:  WorkflowVersionLegacy,
		JobID:    "job-legacy",
		OwnerID:  "member-1",
		Status:   JobDone,
		Proposal: &ProposalRef{ID: "proposal-legacy", Status: ProposalSubmitted},
	}})

	journey, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-legacy")
	if err != nil {
		t.Fatalf("Inspect legacy Journey: %v", err)
	}
	if journey.Version != WorkflowVersion1 || journey.Milestone != MilestoneAwaitingApproval {
		t.Fatalf("legacy Journey = version %d milestone %q", journey.Version, journey.Milestone)
	}
	if len(journey.Attempts) != 0 {
		t.Fatalf("legacy history was invented: %+v", journey.Attempts)
	}
}

func TestWorkflowInspectRejectsUnknownFutureWorkflowVersion(t *testing.T) {
	t.Parallel()

	workflow := newWorkflow(&recordingRepository{record: Record{
		Version: WorkflowVersion1 + 1, JobID: "job-future", OwnerID: "member-1", Status: JobRunning,
	}})
	_, err := workflow.Inspect(context.Background(), Viewer{UserID: "member-1"}, "job-future")
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Inspect future Journey error = %v, want ErrInvalidState", err)
	}
}

func TestWorkflowInspectFailureReturnsSafeGuidanceAndRoleActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		code        FailureCode
		viewer      Viewer
		wantCode    FailureCode
		wantMessage string
		wantActions []Action
	}{
		{
			name: "grounding miss can be edited or retried", code: FailureNoGroundedTitles,
			viewer: Viewer{UserID: "member-1"}, wantCode: FailureNoGroundedTitles,
			wantMessage: "No grounded titles matched this request. Try again, or edit its description and constraints.",
			wantActions: []Action{ActionEdit, ActionRetry},
		},
		{
			name: "provider diagnostic is generalized for member", code: FailureGenerationFailed,
			viewer: Viewer{UserID: "member-1"}, wantCode: FailureGenerationFailed,
			wantMessage: "Loomarr couldn't generate this channel. Try again later.",
			wantActions: []Action{ActionRetry},
		},
		{
			name: "admin may inspect AI configuration", code: FailureGenerationFailed,
			viewer: Viewer{Admin: true}, wantCode: FailureGenerationFailed,
			wantMessage: "Loomarr couldn't generate this channel. Try again later.",
			wantActions: []Action{ActionRetry, ActionCheckAI},
		},
		{
			name: "unknown persisted code fails safe", code: "future_provider_detail",
			viewer: Viewer{UserID: "member-1"}, wantCode: FailureGenerationFailed,
			wantMessage: "Loomarr couldn't generate this channel. Try again later.",
			wantActions: []Action{ActionRetry},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workflow := newWorkflow(&recordingRepository{record: Record{
				Version: WorkflowVersion1,
				JobID:   "job-failed",
				OwnerID: "member-1",
				Status:  JobFailed,
				Attempts: []Attempt{{
					Version: WorkflowVersion1, Number: 1, Status: AttemptFailed,
				}},
				FailureCode: tt.code,
				Diagnostic:  "Post https://provider.example: secret-token connection refused",
			}})

			journey, err := workflow.Inspect(context.Background(), tt.viewer, "job-failed")
			if err != nil {
				t.Fatalf("Inspect failed Journey: %v", err)
			}
			if journey.Milestone != MilestoneFailed {
				t.Fatalf("milestone = %q, want %q", journey.Milestone, MilestoneFailed)
			}
			if journey.Failure == nil || journey.Failure.Code != tt.wantCode || journey.Failure.Message != tt.wantMessage {
				t.Fatalf("failure = %+v, want code %q message %q", journey.Failure, tt.wantCode, tt.wantMessage)
			}
			if len(journey.Actions) != len(tt.wantActions) {
				t.Fatalf("actions = %v, want %v", journey.Actions, tt.wantActions)
			}
			for i := range tt.wantActions {
				if journey.Actions[i] != tt.wantActions[i] {
					t.Fatalf("actions = %v, want %v", journey.Actions, tt.wantActions)
				}
			}
		})
	}
}

type recordingRepository struct {
	record   Record
	err      error
	loadedID string
}

func (r *recordingRepository) Load(_ context.Context, id string) (Record, error) {
	r.loadedID = id
	return r.record, r.err
}
