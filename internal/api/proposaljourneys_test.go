package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/proposalworkflow"
	"github.com/mantonx/loomarr/internal/suggest"
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
		Proposal: &proposalworkflow.ProposalRef{ID: "proposal-1", Status: proposalworkflow.ProposalSubmitted},
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
