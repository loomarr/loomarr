package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestProposalJourneyEndpointReturnsAuthoritativeProjection(t *testing.T) {
	t.Parallel()

	workflow := &fakeProposalWorkflow{journey: proposalworkflow.Journey{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-1",
		Milestone: proposalworkflow.MilestoneAwaitingApproval,
		Intent:    suggest.Intent{Description: "Saturday morning cartoons"},
		Attempts: []proposalworkflow.Attempt{{
			Version: proposalworkflow.WorkflowVersion1, Number: 1, Status: proposalworkflow.AttemptSucceeded,
		}},
		Proposal: &proposalworkflow.ProposalRef{ID: "proposal-1", Status: proposalworkflow.ProposalSubmitted, Proposal: suggest.Proposal{Trace: suggest.DecisionTrace{Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:1", Ownership: "library", Disposition: "selected", Reason: "selected"}}}}},
		Actions:  []proposalworkflow.Action{proposalworkflow.ActionReview},
	}}
	srv := proposalJourneyServer(t, workflow)

	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Journey = %d, want 200", resp.StatusCode)
	}
	var body api.ProposalJourneyDTO
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if workflow.jobID != "job-1" || !workflow.viewer.Admin {
		t.Fatalf("workflow call = viewer %+v job %q", workflow.viewer, workflow.jobID)
	}
	if body.Milestone != "awaiting_approval" || len(body.Attempts) != 1 ||
		body.Proposal == nil || body.Proposal.ID != "proposal-1" ||
		len(body.Actions) != 1 || body.Actions[0] != "review" {
		t.Fatalf("Journey body = %+v", body)
	}
	if body.Proposal.Proposal.Trace.Version != 1 || len(body.Proposal.Proposal.Trace.Candidates) != 1 {
		t.Fatalf("Journey trace = %+v", body.Proposal.Proposal.Trace)
	}
}

func TestProposalJourneyListEndpointUsesCallerScope(t *testing.T) {
	t.Parallel()

	workflow := &fakeProposalWorkflow{journeys: []proposalworkflow.Journey{{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-running",
		Milestone: proposalworkflow.MilestoneGenerating, Intent: suggest.Intent{Description: "Anime after school"},
	}}}
	srv := proposalJourneyServer(t, workflow)
	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs?mine=true&status=generating", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Journeys = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Journeys []api.ProposalJourneyDTO `json:"journeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Journeys) != 1 || body.Journeys[0].JobID != "job-running" {
		t.Fatalf("Journeys = %+v", body.Journeys)
	}
	if workflow.viewer.Admin || !workflow.options.Mine || workflow.options.Milestone != proposalworkflow.MilestoneGenerating {
		t.Fatalf("workflow list call = viewer %+v options %+v", workflow.viewer, workflow.options)
	}
}

func TestProposalJourneyEndpointFailsClosedForForbiddenAndCorruptState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "forbidden", err: proposalworkflow.ErrForbidden, status: http.StatusForbidden},
		{name: "corrupt", err: errors.Join(proposalworkflow.ErrInvalidState, errors.New("private database detail")), status: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := proposalJourneyServer(t, &fakeProposalWorkflow{err: tt.err})
			resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", memberToken, "")
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tt.status {
				t.Fatalf("GET Journey = %d, want %d", resp.StatusCode, tt.status)
			}
		})
	}
}

func TestProposalJourneyFailureProjectionDoesNotSerializePrivateTrace(t *testing.T) {
	t.Parallel()

	private := suggest.DecisionTrace{Version: suggest.DecisionTraceVersion, SurfacedTotal: 65, RecordedTotal: 65, Truncated: true,
		Terminal: suggest.TerminalProviderFailure,
		Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:603", Name: "private-candidate", Ownership: "library",
			Rank: suggest.RankTuple{TieKey: "movie:tmdb:603"}, Disposition: suggest.DispositionSelected, Reason: "selected"}}}
	if err := suggest.ValidateDecisionTrace(private); err != nil {
		t.Fatalf("private trace must be valid: %v", err)
	}
	failure := &proposalworkflow.Failure{Code: proposalworkflow.FailureGenerationFailed, Reason: proposalworkflow.FailureReasonProviderUnavailable,
		RecoveryAction: proposalworkflow.RecoveryActionRetryLater, Message: "safe", Guidance: "safe", Trace: private}
	srv := proposalJourneyServer(t, &fakeProposalWorkflow{journey: proposalworkflow.Journey{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-failed", Milestone: proposalworkflow.MilestoneFailed,
		Failure: failure, Attempts: []proposalworkflow.Attempt{{Version: proposalworkflow.WorkflowVersion1, Number: 1, Status: proposalworkflow.AttemptFailed, Failure: failure}},
	}})
	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-failed", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || containsAny(string(encoded), "private-candidate", "movie:tmdb:603", "library") {
		t.Fatalf("failure response leaked private trace: %s", encoded)
	}
	current, ok := body["failure"].(map[string]any)
	if !ok || current["reason"] != "provider_unavailable" || current["recoveryAction"] != "retry_later" {
		t.Fatalf("current failure recovery = %#v", body["failure"])
	}
	attempts, ok := body["attempts"].([]any)
	if !ok || len(attempts) != 1 {
		t.Fatalf("attempt history = %#v", body["attempts"])
	}
	attempt, ok := attempts[0].(map[string]any)
	if !ok {
		t.Fatalf("attempt = %#v", attempts[0])
	}
	history, ok := attempt["failure"].(map[string]any)
	if !ok || history["reason"] != "provider_unavailable" || history["recoveryAction"] != "retry_later" {
		t.Fatalf("attempt failure recovery = %#v", attempt["failure"])
	}
	if private.Candidates[0].Name != "private-candidate" {
		t.Fatalf("public projection mutated persisted trace: %+v", private)
	}
	for _, projected := range []any{current["trace"], history["trace"]} {
		trace, ok := projected.(map[string]any)
		if !ok || trace["terminal"] != suggest.TerminalProviderFailure || trace["truncated"] != true || trace["surfacedTotal"] != float64(65) || trace["recordedTotal"] != float64(65) || trace["candidates"] != nil || len(trace) != 6 {
			t.Fatalf("public trace = %#v", projected)
		}
	}
}

func containsAny(value string, values ...string) bool {
	for _, candidate := range values {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

type fakeProposalWorkflow struct {
	journey  proposalworkflow.Journey
	journeys []proposalworkflow.Journey
	err      error
	viewer   proposalworkflow.Viewer
	jobID    string
	options  proposalworkflow.ListOptions
}

func (f *fakeProposalWorkflow) List(
	_ context.Context,
	viewer proposalworkflow.Viewer,
	options proposalworkflow.ListOptions,
) ([]proposalworkflow.Journey, error) {
	f.viewer, f.options = viewer, options
	return f.journeys, f.err
}

func (f *fakeProposalWorkflow) Inspect(
	_ context.Context,
	viewer proposalworkflow.Viewer,
	jobID string,
) (proposalworkflow.Journey, error) {
	f.viewer, f.jobID = viewer, jobID
	return f.journey, f.err
}

func proposalJourneyServer(t *testing.T, workflow api.ProposalWorkflow) *httptest.Server {
	t.Helper()
	handler := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth: testAuthorizer{}, Log: slog.New(slog.DiscardHandler), ProposalWorkflow: workflow,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
