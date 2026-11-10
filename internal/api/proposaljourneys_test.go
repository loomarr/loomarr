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
	"time"

	"github.com/loomarr/loomarr/internal/api"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
)

func TestProposalJourneyEndpointReturnsAuthoritativeProjection(t *testing.T) {
	t.Parallel()

	flow := &fakeProposalWorkflow{journey: workflow.Journey{
		Version: workflow.WorkflowVersion1, JobID: "job-1",
		Milestone: workflow.MilestoneAwaitingApproval,
		Intent:    proposals.Intent{Description: "Saturday morning cartoons"},
		Attempts: []workflow.Attempt{{
			Version: workflow.WorkflowVersion1, Number: 1, Status: workflow.AttemptSucceeded,
		}},
		Proposal: &workflow.ProposalRef{ID: "proposal-1", Status: workflow.ProposalSubmitted, Proposal: proposals.Proposal{Trace: proposals.DecisionTrace{Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []proposals.DecisionCandidate{{Key: "movie:tmdb:90000001", Ownership: "library", Disposition: "selected", Reason: "selected"}}}}},
		Actions:  []workflow.Action{workflow.ActionReview},
	}}
	srv := proposalJourneyServer(t, flow)

	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Journey = %d, want 200", resp.StatusCode)
	}
	var body proposalsapi.ProposalJourneyDTO
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if flow.jobID != "job-1" || !flow.viewer.Admin {
		t.Fatalf("workflow call = viewer %+v job %q", flow.viewer, flow.jobID)
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

func TestReviseProposalJourneyKeepsTheStableJob(t *testing.T) {
	t.Parallel()

	flow := &fakeProposalWorkflow{}
	srv := proposalJourneyServer(t, flow)
	resp := do(t, srv, http.MethodPost, "/v1/proposal-jobs/job-1/revise", adminToken,
		`{"description":"80s comedies with more variety","runtimeTargetMin":240}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST revision = %d, want 200", resp.StatusCode)
	}
	var body struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.JobID != "job-1" || flow.jobID != "job-1" || !flow.viewer.Admin ||
		flow.intent.Description != "80s comedies with more variety" || flow.intent.RuntimeTgt != 240 {
		t.Fatalf("revision call = body %+v, viewer %+v, job %q, intent %+v",
			body, flow.viewer, flow.jobID, flow.intent)
	}
}

func TestReviseProposalJourneyRequiresAdminAndCurrentReview(t *testing.T) {
	t.Parallel()

	flow := &fakeProposalWorkflow{}
	srv := proposalJourneyServer(t, flow)
	member := do(t, srv, http.MethodPost, "/v1/proposal-jobs/job-1/revise", memberToken,
		`{"description":"more variety"}`)
	defer func() { _ = member.Body.Close() }()
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member revision = %d, want 403", member.StatusCode)
	}

	flow.err = workflow.ErrNotRevisable
	stale := do(t, srv, http.MethodPost, "/v1/proposal-jobs/job-1/revise", adminToken,
		`{"description":"more variety"}`)
	defer func() { _ = stale.Body.Close() }()
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale revision = %d, want 409", stale.StatusCode)
	}
}

func TestProposalJourneyListEndpointUsesCallerScope(t *testing.T) {
	t.Parallel()

	flow := &fakeProposalWorkflow{journeys: []workflow.Journey{{
		Version: workflow.WorkflowVersion1, JobID: "job-running",
		Milestone: workflow.MilestoneGenerating, Intent: proposals.Intent{Description: "Anime after school"},
	}}}
	srv := proposalJourneyServer(t, flow)
	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs?mine=true&status=generating", memberToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Journeys = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Journeys []proposalsapi.ProposalJourneyDTO `json:"journeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Journeys) != 1 || body.Journeys[0].JobID != "job-running" {
		t.Fatalf("Journeys = %+v", body.Journeys)
	}
	if flow.viewer.Admin || !flow.options.Mine || flow.options.Milestone != workflow.MilestoneGenerating {
		t.Fatalf("workflow list call = viewer %+v options %+v", flow.viewer, flow.options)
	}
}

func TestProposalJourneyEndpointFailsClosedForForbiddenAndCorruptState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "forbidden", err: workflow.ErrForbidden, status: http.StatusForbidden},
		{name: "corrupt", err: errors.Join(workflow.ErrInvalidState, errors.New("private database detail")), status: http.StatusInternalServerError},
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

	private := proposals.DecisionTrace{Version: proposals.DecisionTraceVersion, SurfacedTotal: 65, RecordedTotal: 65, Truncated: true,
		Terminal: proposals.TerminalProviderFailure,
		Candidates: []proposals.DecisionCandidate{{Key: "movie:tmdb:90000603", Name: "private-candidate", Ownership: "library",
			Rank: proposals.RankTuple{TieKey: "movie:tmdb:90000603"}, Disposition: proposals.DispositionSelected, Reason: "selected"}}}
	if err := proposals.ValidateDecisionTrace(private); err != nil {
		t.Fatalf("private trace must be valid: %v", err)
	}
	failure := &workflow.Failure{Code: workflow.FailureGenerationFailed, Reason: workflow.FailureReasonProviderUnavailable,
		RecoveryAction: workflow.RecoveryActionRetryLater, Message: "safe", Guidance: "safe", Trace: private}
	srv := proposalJourneyServer(t, &fakeProposalWorkflow{journey: workflow.Journey{
		Version: workflow.WorkflowVersion1, JobID: "job-failed", Milestone: workflow.MilestoneFailed,
		Failure: failure, Attempts: []workflow.Attempt{{Version: workflow.WorkflowVersion1, Number: 1, Status: workflow.AttemptFailed, Failure: failure}},
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
	if string(encoded) == "" || containsAny(string(encoded), "private-candidate", "movie:tmdb:90000603", "library") {
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
		// No candidate leaves the server; `candidates` is a required array, so it arrives empty.
		candidates, isList := trace["candidates"].([]any)
		if !ok || trace["terminal"] != proposals.TerminalProviderFailure || trace["truncated"] != true || trace["surfacedTotal"] != float64(65) || trace["recordedTotal"] != float64(65) || !isList || len(candidates) != 0 || len(trace) != 6 {
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
	journey  workflow.Journey
	journeys []workflow.Journey
	err      error
	viewer   workflow.Viewer
	jobID    string
	options  workflow.ListOptions
	intent   proposals.Intent
}

func (f *fakeProposalWorkflow) List(
	_ context.Context,
	viewer workflow.Viewer,
	options workflow.ListOptions,
) ([]workflow.Journey, error) {
	f.viewer, f.options = viewer, options
	return f.journeys, f.err
}

func (f *fakeProposalWorkflow) Inspect(
	_ context.Context,
	viewer workflow.Viewer,
	jobID string,
) (workflow.Journey, error) {
	f.viewer, f.jobID = viewer, jobID
	return f.journey, f.err
}

func (f *fakeProposalWorkflow) Revise(
	_ context.Context,
	viewer workflow.Viewer,
	jobID string,
	intent proposals.Intent,
) error {
	f.viewer, f.jobID, f.intent = viewer, jobID, intent
	return f.err
}

func proposalJourneyServer(t *testing.T, workflow proposalsapi.ProposalWorkflow, configure ...func(*api.Options)) *httptest.Server {
	t.Helper()
	opts := api.Options{Auth: testAuthorizer{}, Log: slog.New(slog.DiscardHandler), ProposalWorkflow: workflow}
	for _, fn := range configure {
		fn(&opts)
	}
	handler := api.Router(slog.New(slog.DiscardHandler), opts)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// The Requests detail says who approved a request and when (#1405). The Journey carried only the
// approver's raw id and no time, so the page could not show either without a second, unscoped
// call to the proposals list.
func TestProposalJourneyReportsWhoApprovedAndWhen(t *testing.T) {
	t.Parallel()

	approvedAt := time.Date(2026, 9, 24, 18, 30, 0, 0, time.UTC)
	flow := &fakeProposalWorkflow{journey: workflow.Journey{
		Version: workflow.WorkflowVersion1, JobID: "job-1",
		Milestone: workflow.MilestoneLive,
		Intent:    proposals.Intent{Description: "Saturday morning cartoons"},
		Proposal: &workflow.ProposalRef{
			ID: "proposal-1", Status: workflow.ProposalApproved,
			ApprovedBy: proposalstore.AutoApprovedBy, ApprovedAt: approvedAt,
		},
	}}
	srv := proposalJourneyServer(t, flow)

	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body proposalsapi.ProposalJourneyDTO
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Proposal == nil || body.Proposal.ApprovedAt != "2026-09-24T18:30:00Z" ||
		body.Proposal.ApprovedByName != "Automatic" {
		t.Fatalf("Journey proposal = %+v, want approvedAt 2026-09-24T18:30:00Z by Automatic", body.Proposal)
	}
}

type fakeProgress struct {
	snap suggest.ProgressSnapshot
	ok   bool
	got  string
}

func (f *fakeProgress) Progress(jobID string) (suggest.ProgressSnapshot, bool) {
	f.got = jobID
	return f.snap, f.ok
}

func getJourney(t *testing.T, srv *httptest.Server) proposalsapi.ProposalJourneyDTO {
	t.Helper()
	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Journey = %d, want 200", resp.StatusCode)
	}
	var body proposalsapi.ProposalJourneyDTO
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

// A generating Journey carries the live snapshot, so a reload or another device sees the same
// stage line and streamed titles the first tab does.
func TestProposalJourneyCarriesLiveProgressWhileGenerating(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	progress := &fakeProgress{ok: true, snap: suggest.ProgressSnapshot{
		Stage: suggest.StageChoosing, Terms: []string{"engines"}, Target: 8, StartedAt: started,
		Picks: []suggest.ProgressPick{{Key: "movie:tmdb:90000100", MediaType: "movie", Name: "Quiet Engines", Year: 1994, TMDBID: 90000100, InLibrary: false}},
	}}
	wf := &fakeProposalWorkflow{journey: workflow.Journey{
		Version: workflow.WorkflowVersion1, JobID: "job-1", Milestone: workflow.MilestoneGenerating,
	}}
	body := getJourney(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress }))
	if progress.got != "job-1" {
		t.Fatalf("progress read for %q, want job-1", progress.got)
	}
	p := body.Progress
	if p == nil || p.Stage != "choosing" || p.Target != 8 || len(p.Terms) != 1 || p.Terms[0] != "engines" ||
		len(p.Picks) != 1 || p.Picks[0].Name != "Quiet Engines" || p.Picks[0].Year != 1994 || !p.StartedAt.Equal(started) {
		t.Fatalf("progress = %+v", p)
	}
}

// Once the run has ended the Proposal or the failure is the record: no progress, even if a
// stale snapshot were still around.
func TestProposalJourneyOmitsProgressOutsideGenerating(t *testing.T) {
	t.Parallel()
	progress := &fakeProgress{ok: true, snap: suggest.ProgressSnapshot{Stage: suggest.StageChoosing}}
	wf := &fakeProposalWorkflow{journey: workflow.Journey{
		Version: workflow.WorkflowVersion1, JobID: "job-1", Milestone: workflow.MilestoneAwaitingApproval,
	}}
	if body := getJourney(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress })); body.Progress != nil {
		t.Fatalf("progress = %+v on an awaiting_approval journey, want none", body.Progress)
	}
	// A queued job (not running on this process) has no snapshot: progress is absent, not empty.
	progress.ok = false
	wf.journey.Milestone = workflow.MilestoneGenerating
	if body := getJourney(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress })); body.Progress != nil {
		t.Fatalf("progress = %+v for a job with no live run, want none", body.Progress)
	}
}

// The Requests page lists journeys, and its In progress card shows the same stage line.
func TestProposalJourneyListCarriesLiveProgressToo(t *testing.T) {
	t.Parallel()
	progress := &fakeProgress{ok: true, snap: suggest.ProgressSnapshot{Stage: suggest.StageSearching, Terms: []string{"engines"}, Target: 8}}
	wf := &fakeProposalWorkflow{journeys: []workflow.Journey{{
		Version: workflow.WorkflowVersion1, JobID: "job-1", Milestone: workflow.MilestoneGenerating,
	}}}
	resp := do(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress }), http.MethodGet, "/v1/proposal-jobs", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Journeys []proposalsapi.ProposalJourneyDTO `json:"journeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Journeys) != 1 || body.Journeys[0].Progress == nil || body.Journeys[0].Progress.Stage != "searching" {
		t.Fatalf("journeys = %+v, want the searching stage on the generating one", body.Journeys)
	}
}
