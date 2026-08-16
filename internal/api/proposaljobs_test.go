package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

const (
	aliceToken        = "proposal-job-alice"
	bobToken          = "proposal-job-bob"
	sessionAdminToken = "proposal-job-admin"
)

// proposalJobAuthorizer gives the HTTP seam real caller identities. A role-only
// authorizer cannot prove the caller-owned resource rules these routes exist to
// enforce.
type proposalJobAuthorizer struct{}

func (proposalJobAuthorizer) Authorize(r *http.Request) api.Role {
	role, _ := (proposalJobAuthorizer{}).AuthorizeUser(r)
	return role
}

func (proposalJobAuthorizer) AuthorizeUser(r *http.Request) (api.Role, *store.User) {
	switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
	case aliceToken:
		return api.RoleMember, &store.User{ID: "alice", Role: store.RoleMember}
	case bobToken:
		return api.RoleMember, &store.User{ID: "bob", Role: store.RoleMember}
	case sessionAdminToken:
		return api.RoleAdmin, &store.User{ID: "admin", Role: store.RoleAdmin}
	case adminToken:
		return api.RoleAdmin, nil // API_TOKEN: privileged, but not a person.
	default:
		return api.RoleAnonymous, nil
	}
}

func newProposalJobServer(t *testing.T) (*httptest.Server, store.Store) {
	t.Helper()
	st := openTestStore(t, filepath.Join(t.TempDir(), "proposal-jobs.db"))
	t.Cleanup(func() { _ = st.Close() })
	log := slog.New(slog.DiscardHandler)
	srv := httptest.NewServer(api.Router(log, api.Options{
		Store: st,
		Auth:  proposalJobAuthorizer{},
		Log:   log,
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

func createProposalJob(t *testing.T, st store.Store, job store.Job) {
	t.Helper()
	if err := st.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

type proposalJobResponse struct {
	JobID     string         `json:"jobId"`
	Status    string         `json:"status"`
	Intent    suggest.Intent `json:"intent"`
	Attempts  int            `json:"attempts"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
	Failure   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"failure,omitempty"`
	Proposal *api.ProposalDTO `json:"proposal,omitempty"`
}

type proposalJobsResponse struct {
	ProposalJobs []proposalJobResponse `json:"proposalJobs"`
}

func decodeProposalJob(t *testing.T, resp *http.Response) proposalJobResponse {
	t.Helper()
	defer resp.Body.Close()
	var got proposalJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestGetProposalJobRestoresOwnedQueuedIntent(t *testing.T) {
	srv, st := newProposalJobServer(t)
	created := time.Date(2026, time.August, 15, 14, 30, 0, 0, time.FixedZone("EDT", -4*60*60))
	updated := created.Add(45 * time.Second)
	wantIntent := suggest.Intent{
		Description: "90s cartoons without anything scary",
		Era:         "1990s",
		Tone:        "playful",
		RuntimeTgt:  180,
		MustInclude: []string{"Batman"},
		MustExclude: []string{"horror"},
		MaxAcquire:  3,
		RefineText:  "more weekday-afternoon energy",
		CurrentLineup: []suggest.LineupContext{{
			Name: "Animaniacs", Year: 1993, Key: "series:tvdb:72879",
		}},
		Adjacent: []suggest.AdjacentContext{{
			Name: "Pinky and the Brain", Year: 1995, Key: "series:tvdb:73787", Votes: 4,
		}},
	}
	intentJSON, err := json.Marshal(wantIntent)
	if err != nil {
		t.Fatal(err)
	}
	createProposalJob(t, st, store.Job{
		ID: "job-queued", Kind: "suggest", Status: "queued", IntentJSON: string(intentJSON),
		IntentHash: "intent-hash", CreatedBy: "alice", Attempts: 2,
		Deadline: created, CreatedAt: created, UpdatedAt: updated,
	})

	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-queued", aliceToken, "")
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("owned queued proposal job -> %d, want 200", resp.StatusCode)
	}
	got := decodeProposalJob(t, resp)
	if got.JobID != "job-queued" || got.Status != "queued" || got.Attempts != 2 {
		t.Errorf("lifecycle = (%q, %q, %d), want (job-queued, queued, 2)", got.JobID, got.Status, got.Attempts)
	}
	wantCreated := created.UTC().Format(time.RFC3339)
	wantUpdated := updated.UTC().Format(time.RFC3339)
	if got.CreatedAt != wantCreated || got.UpdatedAt != wantUpdated {
		t.Errorf("timestamps = (%q, %q), want (%q, %q)", got.CreatedAt, got.UpdatedAt, wantCreated, wantUpdated)
	}
	if !intentsEqual(got.Intent, wantIntent) {
		t.Errorf("intent = %#v, want %#v", got.Intent, wantIntent)
	}
	if got.Failure != nil || got.Proposal != nil {
		t.Errorf("queued job fabricated terminal artifacts: failure=%+v proposal=%+v", got.Failure, got.Proposal)
	}
}

func TestGetProposalJobEnforcesCallerOwnership(t *testing.T) {
	srv, st := newProposalJobServer(t)
	now := time.Date(2026, time.August, 15, 18, 0, 0, 0, time.UTC)
	createProposalJob(t, st, store.Job{
		ID: "alice-job", Kind: "suggest", Status: "running",
		IntentJSON: `{"description":"cozy mysteries"}`, IntentHash: "alice-hash",
		CreatedBy: "alice", Attempts: 1, Deadline: now, CreatedAt: now, UpdatedAt: now,
	})

	tests := []struct {
		name  string
		path  string
		token string
		want  int
	}{
		{name: "owner member", path: "/v1/proposal-jobs/alice-job", token: aliceToken, want: http.StatusOK},
		{name: "other member", path: "/v1/proposal-jobs/alice-job", token: bobToken, want: http.StatusForbidden},
		{name: "session admin", path: "/v1/proposal-jobs/alice-job", token: sessionAdminToken, want: http.StatusOK},
		{name: "API token admin", path: "/v1/proposal-jobs/alice-job", token: adminToken, want: http.StatusOK},
		{name: "anonymous", path: "/v1/proposal-jobs/alice-job", want: http.StatusUnauthorized},
		{name: "missing", path: "/v1/proposal-jobs/missing", token: aliceToken, want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, srv, http.MethodGet, tc.path, tc.token, "")
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("GET %s as %q -> %d, want %d", tc.path, tc.token, resp.StatusCode, tc.want)
			}
		})
	}
}

func TestGetProposalJobReturnsNewestProposalInAnyDecisionState(t *testing.T) {
	srv, st := newProposalJobServer(t)
	now := time.Date(2026, time.August, 15, 19, 0, 0, 0, time.UTC)
	createProposalJob(t, st, store.Job{
		ID: "job-decided", Kind: "suggest", Status: "done",
		IntentJSON: `{"description":"late-night science fiction","tone":"atmospheric"}`,
		IntentHash: "decided-hash", CreatedBy: "alice", Attempts: 1,
		Deadline: now, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	})
	for _, proposal := range []store.Proposal{
		{
			ID: "proposal-submitted", JobID: "job-decided", Status: "submitted", CreatedBy: "alice",
			ProposalJSON: `{"intent":{"description":"old submitted result"},"lineup":[],"acquisitions":[],"alternates":[],"scores":{}}`,
			CreatedAt:    now, UpdatedAt: now,
		},
		{
			ID: "proposal-denied", JobID: "job-decided", Status: "denied", CreatedBy: "alice",
			DenyReason: "Too broad", Note: "Narrow this to first-contact stories.",
			ProposalJSON: `{"intent":{"description":"late-night science fiction","tone":"atmospheric"},"lineup":[],"acquisitions":[],"alternates":[],"scores":{},"rationale":"Atmospheric first-contact stories."}`,
			CreatedAt:    now.Add(time.Minute), UpdatedAt: now.Add(2 * time.Minute),
		},
	} {
		if err := st.CreateProposal(context.Background(), proposal); err != nil {
			t.Fatal(err)
		}
	}

	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-decided", aliceToken, "")
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("done job -> %d, want 200", resp.StatusCode)
	}
	got := decodeProposalJob(t, resp)
	if got.Status != "done" || got.Proposal == nil {
		t.Fatalf("done projection = status %q proposal %+v", got.Status, got.Proposal)
	}
	if got.Proposal.ID != "proposal-denied" || got.Proposal.Status != "denied" {
		t.Errorf("newest proposal = (%q, %q), want denied proposal", got.Proposal.ID, got.Proposal.Status)
	}
	if got.Proposal.DenyReason != "Too broad" || got.Proposal.Note != "Narrow this to first-contact stories." {
		t.Errorf("decision provenance was lost: %+v", got.Proposal)
	}
	if got.Proposal.Proposal.Rationale != "Atmospheric first-contact stories." {
		t.Errorf("typed proposal rationale = %q", got.Proposal.Proposal.Rationale)
	}
	if got.Failure != nil {
		t.Errorf("done job exposed failure %+v", got.Failure)
	}

	createProposalJob(t, st, store.Job{
		ID: "job-approved", Kind: "suggest", Status: "done",
		IntentJSON: `{"description":"cozy mysteries"}`, IntentHash: "approved-hash",
		CreatedBy: "alice", Attempts: 2, Deadline: now, CreatedAt: now, UpdatedAt: now,
	})
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: "proposal-approved", JobID: "job-approved", Status: "approved", CreatedBy: "alice",
		ApprovedBy: "admin", ModSummary: "dropped 1, added 1", Note: "Kept this family-friendly.",
		ProposalJSON: `{"intent":{"description":"cozy mysteries"},"lineup":[],"acquisitions":[],"alternates":[],"scores":{}}`,
		ApprovedAt:   now.Add(3 * time.Minute), CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	approvedResp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-approved", aliceToken, "")
	if approvedResp.StatusCode != http.StatusOK {
		defer approvedResp.Body.Close()
		t.Fatalf("approved job -> %d, want 200", approvedResp.StatusCode)
	}
	approved := decodeProposalJob(t, approvedResp)
	if approved.Proposal == nil || approved.Proposal.Status != "approved" ||
		approved.Proposal.ApprovedBy != "admin" || approved.Proposal.ModSummary != "dropped 1, added 1" ||
		approved.Proposal.Note != "Kept this family-friendly." || approved.Proposal.ApprovedAt == "" {
		t.Errorf("approved proposal decision was not restored: %+v", approved.Proposal)
	}
}

func TestGetProposalJobRunningHasNoTerminalArtifacts(t *testing.T) {
	srv, st := newProposalJobServer(t)
	now := time.Date(2026, time.August, 15, 19, 30, 0, 0, time.UTC)
	createProposalJob(t, st, store.Job{
		ID: "job-running", Kind: "suggest", Status: "running",
		IntentJSON: `{"description":"action marathon"}`, IntentHash: "running-hash",
		CreatedBy: "alice", FailureCode: "generation_failed", LastError: "stale prior run",
		Attempts: 4, Deadline: now, CreatedAt: now, UpdatedAt: now,
	})
	resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/job-running", aliceToken, "")
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("running job -> %d, want 200", resp.StatusCode)
	}
	got := decodeProposalJob(t, resp)
	if got.Status != "running" || got.Failure != nil || got.Proposal != nil {
		t.Errorf("running projection = status %q failure %+v proposal %+v", got.Status, got.Failure, got.Proposal)
	}
}

func TestGetProposalJobMapsFailureCodeWithoutLeakingDiagnostic(t *testing.T) {
	srv, st := newProposalJobServer(t)
	now := time.Date(2026, time.August, 15, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		storedCode  string
		wantCode    string
		wantMessage string
	}{
		{
			storedCode: "no_grounded_titles", wantCode: "no_grounded_titles",
			wantMessage: "No grounded titles matched this request. Edit the channel description or constraints and try again.",
		},
		{
			storedCode: "timed_out", wantCode: "timed_out",
			wantMessage: "Channel generation took too long. Try again.",
		},
		{
			storedCode: "provider_unavailable", wantCode: "provider_unavailable",
			wantMessage: "The AI provider is unavailable right now. Check the AI connection or try again later.",
		},
		{
			storedCode: "generation_failed", wantCode: "generation_failed",
			wantMessage: "Loomarr couldn't generate this channel. Try again.",
		},
		{
			storedCode: "legacy_unbounded_value", wantCode: "generation_failed",
			wantMessage: "Loomarr couldn't generate this channel. Try again.",
		},
	}
	for i, tc := range tests {
		id := fmt.Sprintf("failed-%d", i)
		createProposalJob(t, st, store.Job{
			ID: id, Kind: "suggest", Status: "failed",
			IntentJSON: `{"description":"Saturday cartoons"}`, IntentHash: "failed-hash-" + id,
			CreatedBy: "alice", FailureCode: tc.storedCode,
			LastError: "provider returned api_key=do-not-leak and raw upstream body",
			Attempts:  3, Deadline: now, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		})

		resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs/"+id, aliceToken, "")
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			t.Fatalf("failure %q -> %d, want 200", tc.storedCode, resp.StatusCode)
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "do-not-leak") || strings.Contains(string(raw), "raw upstream") {
			t.Fatalf("private diagnostic crossed the API boundary: %s", raw)
		}
		var got proposalJobResponse
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got.Failure == nil {
			t.Fatalf("failure %q omitted failure body: %s", tc.storedCode, raw)
		}
		if got.Failure.Code != tc.wantCode || got.Failure.Message != tc.wantMessage {
			t.Errorf("failure %q = (%q, %q), want (%q, %q)", tc.storedCode,
				got.Failure.Code, got.Failure.Message, tc.wantCode, tc.wantMessage)
		}
		if got.Proposal != nil {
			t.Errorf("failed job fabricated proposal %+v", got.Proposal)
		}
	}
}

func TestListProposalJobsScopesAndFiltersAuthoritativeHistory(t *testing.T) {
	srv, st := newProposalJobServer(t)
	now := time.Date(2026, time.August, 15, 21, 0, 0, 0, time.UTC)
	jobs := []store.Job{
		{ID: "alice-queued", Kind: "suggest", Status: "queued", IntentJSON: `{"description":"Alice queued"}`, IntentHash: "h-aq", CreatedBy: "alice", CreatedAt: now, UpdatedAt: now},
		{ID: "alice-done", Kind: "suggest", Status: "done", IntentJSON: `{"description":"Alice done"}`, IntentHash: "h-ad", CreatedBy: "alice", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
		{ID: "bob-running", Kind: "suggest", Status: "running", IntentJSON: `{"description":"Bob running"}`, IntentHash: "h-br", CreatedBy: "bob", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
		{ID: "admin-failed", Kind: "suggest", Status: "failed", IntentJSON: `{"description":"Admin failed"}`, IntentHash: "h-af", CreatedBy: "admin", FailureCode: "timed_out", LastError: "private", CreatedAt: now.Add(3 * time.Minute), UpdatedAt: now.Add(3 * time.Minute)},
		// A token-created row still does not make API_TOKEN a person with a meaningful `mine`.
		{ID: "token-done", Kind: "suggest", Status: "done", IntentJSON: `{"description":"Token done"}`, IntentHash: "h-td", CreatedBy: "", CreatedAt: now.Add(4 * time.Minute), UpdatedAt: now.Add(4 * time.Minute)},
	}
	for _, job := range jobs {
		createProposalJob(t, st, job)
	}
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: "alice-submitted", JobID: "alice-done", Status: "submitted", CreatedBy: "alice",
		ProposalJSON: `{"intent":{"description":"Alice done"},"lineup":[],"acquisitions":[],"alternates":[],"scores":{}}`,
		CreatedAt:    now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	assertIDs := func(t *testing.T, path, token string, want ...string) {
		t.Helper()
		resp := do(t, srv, http.MethodGet, path, token, "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s -> %d, want 200", path, resp.StatusCode)
		}
		var body proposalJobsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(body.ProposalJobs))
		for _, job := range body.ProposalJobs {
			got = append(got, job.JobID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("GET %s ids = %v, want %v", path, got, want)
		}
	}

	t.Run("member mine", func(t *testing.T) {
		assertIDs(t, "/v1/proposal-jobs?mine=true", aliceToken, "alice-done", "alice-queued")
	})
	t.Run("member is scoped even without mine", func(t *testing.T) {
		assertIDs(t, "/v1/proposal-jobs?user=bob", aliceToken, "alice-done", "alice-queued")
	})
	t.Run("member lifecycle filter", func(t *testing.T) {
		resp := do(t, srv, http.MethodGet, "/v1/proposal-jobs?mine=true&status=done", aliceToken, "")
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			t.Fatalf("member done history -> %d", resp.StatusCode)
		}
		var body proposalJobsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if len(body.ProposalJobs) != 1 || body.ProposalJobs[0].Proposal == nil ||
			body.ProposalJobs[0].Proposal.Status != "submitted" {
			t.Fatalf("done history lost submitted proposal: %+v", body.ProposalJobs)
		}
	})
	t.Run("session admin mine", func(t *testing.T) {
		assertIDs(t, "/v1/proposal-jobs?mine=true", sessionAdminToken, "admin-failed")
	})
	t.Run("session admin diagnoses all", func(t *testing.T) {
		assertIDs(t, "/v1/proposal-jobs", sessionAdminToken,
			"token-done", "admin-failed", "bob-running", "alice-done", "alice-queued")
	})
	t.Run("API token mine is empty", func(t *testing.T) {
		assertIDs(t, "/v1/proposal-jobs?mine=true", adminToken)
	})
	t.Run("API token diagnoses all", func(t *testing.T) {
		assertIDs(t, "/v1/proposal-jobs?limit=1", adminToken, "token-done")
	})
}

func TestListProposalJobsRejectsInvalidBounds(t *testing.T) {
	srv, _ := newProposalJobServer(t)
	for _, path := range []string{
		"/v1/proposal-jobs?status=not-a-lifecycle",
		"/v1/proposal-jobs?limit=-1",
		"/v1/proposal-jobs?limit=101",
	} {
		resp := do(t, srv, http.MethodGet, path, adminToken, "")
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("GET %s -> %d, want 422", path, resp.StatusCode)
		}
	}
}

func intentsEqual(a, b suggest.Intent) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
