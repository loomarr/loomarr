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

func TestReviseProposalJourneyKeepsTheStableJob(t *testing.T) {
	t.Parallel()

	workflow := &fakeProposalWorkflow{}
	srv := proposalJourneyServer(t, workflow)
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
	if body.JobID != "job-1" || workflow.jobID != "job-1" || !workflow.viewer.Admin ||
		workflow.intent.Description != "80s comedies with more variety" || workflow.intent.RuntimeTgt != 240 {
		t.Fatalf("revision call = body %+v, viewer %+v, job %q, intent %+v",
			body, workflow.viewer, workflow.jobID, workflow.intent)
	}
}

func TestReviseProposalJourneyRequiresAdminAndCurrentReview(t *testing.T) {
	t.Parallel()

	workflow := &fakeProposalWorkflow{}
	srv := proposalJourneyServer(t, workflow)
	member := do(t, srv, http.MethodPost, "/v1/proposal-jobs/job-1/revise", memberToken,
		`{"description":"more variety"}`)
	defer func() { _ = member.Body.Close() }()
	if member.StatusCode != http.StatusForbidden {
		t.Fatalf("member revision = %d, want 403", member.StatusCode)
	}

	workflow.err = proposalworkflow.ErrNotRevisable
	stale := do(t, srv, http.MethodPost, "/v1/proposal-jobs/job-1/revise", adminToken,
		`{"description":"more variety"}`)
	defer func() { _ = stale.Body.Close() }()
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale revision = %d, want 409", stale.StatusCode)
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
	intent   suggest.Intent
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

func (f *fakeProposalWorkflow) Revise(
	_ context.Context,
	viewer proposalworkflow.Viewer,
	jobID string,
	intent suggest.Intent,
) error {
	f.viewer, f.jobID, f.intent = viewer, jobID, intent
	return f.err
}

func proposalJourneyServer(t *testing.T, workflow api.ProposalWorkflow, configure ...func(*api.Options)) *httptest.Server {
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
	workflow := &fakeProposalWorkflow{journey: proposalworkflow.Journey{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-1",
		Milestone: proposalworkflow.MilestoneLive,
		Intent:    suggest.Intent{Description: "Saturday morning cartoons"},
		Proposal: &proposalworkflow.ProposalRef{
			ID: "proposal-1", Status: proposalworkflow.ProposalApproved,
			ApprovedBy: suggest.AutoApprovedBy, ApprovedAt: approvedAt,
		},
	}}
	srv := proposalJourneyServer(t, workflow)

	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body api.ProposalJourneyDTO
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

func getJourney(t *testing.T, srv *httptest.Server) api.ProposalJourneyDTO {
	t.Helper()
	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET Journey = %d, want 200", resp.StatusCode)
	}
	var body api.ProposalJourneyDTO
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
		Stage: suggest.StageChoosing, Terms: []string{"speed"}, Target: 8, StartedAt: started,
		Picks: []suggest.ProgressPick{{Key: "movie:tmdb:100", MediaType: "movie", Name: "Speed", Year: 1994, TMDBID: 100, InLibrary: false}},
	}}
	wf := &fakeProposalWorkflow{journey: proposalworkflow.Journey{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-1", Milestone: proposalworkflow.MilestoneGenerating,
	}}
	body := getJourney(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress }))
	if progress.got != "job-1" {
		t.Fatalf("progress read for %q, want job-1", progress.got)
	}
	p := body.Progress
	if p == nil || p.Stage != "choosing" || p.Target != 8 || len(p.Terms) != 1 || p.Terms[0] != "speed" ||
		len(p.Picks) != 1 || p.Picks[0].Name != "Speed" || p.Picks[0].Year != 1994 || !p.StartedAt.Equal(started) {
		t.Fatalf("progress = %+v", p)
	}
}

// Once the run has ended the Proposal or the failure is the record: no progress, even if a
// stale snapshot were still around.
func TestProposalJourneyOmitsProgressOutsideGenerating(t *testing.T) {
	t.Parallel()
	progress := &fakeProgress{ok: true, snap: suggest.ProgressSnapshot{Stage: suggest.StageChoosing}}
	wf := &fakeProposalWorkflow{journey: proposalworkflow.Journey{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-1", Milestone: proposalworkflow.MilestoneAwaitingApproval,
	}}
	if body := getJourney(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress })); body.Progress != nil {
		t.Fatalf("progress = %+v on an awaiting_approval journey, want none", body.Progress)
	}
	// A queued job (not running on this process) has no snapshot: progress is absent, not empty.
	progress.ok = false
	wf.journey.Milestone = proposalworkflow.MilestoneGenerating
	if body := getJourney(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress })); body.Progress != nil {
		t.Fatalf("progress = %+v for a job with no live run, want none", body.Progress)
	}
}

// The Requests page lists journeys, and its In progress card shows the same stage line.
func TestProposalJourneyListCarriesLiveProgressToo(t *testing.T) {
	t.Parallel()
	progress := &fakeProgress{ok: true, snap: suggest.ProgressSnapshot{Stage: suggest.StageSearching, Terms: []string{"speed"}, Target: 8}}
	wf := &fakeProposalWorkflow{journeys: []proposalworkflow.Journey{{
		Version: proposalworkflow.WorkflowVersion1, JobID: "job-1", Milestone: proposalworkflow.MilestoneGenerating,
	}}}
	resp := do(t, proposalJourneyServer(t, wf, func(o *api.Options) { o.ProposalProgress = progress }), http.MethodGet, "/v1/proposal-jobs", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Journeys []api.ProposalJourneyDTO `json:"journeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Journeys) != 1 || body.Journeys[0].Progress == nil || body.Journeys[0].Progress.Stage != "searching" {
		t.Fatalf("journeys = %+v, want the searching stage on the generating one", body.Journeys)
	}
}
