package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeSuggest records submissions.
type fakeSuggest struct{ submits int }

func (f *fakeSuggest) Submit(_ context.Context, desc, era, tone string, inc, exc []string, max int, createdBy string) (string, error) {
	f.submits++
	return "job-1", nil
}

// fakeSearch returns a fixed candidate.
type fakeSearch struct{}

func (fakeSearch) Search(_ context.Context, q, scope string, limit int) ([]api.SearchCandidate, error) {
	return []api.SearchCandidate{{MediaType: "movie", TMDBID: 603, Name: "The Matrix", InLibrary: true}}, nil
}

func newSuggestServer(t *testing.T) (*httptest.Server, store.Store, *fakeSuggest) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/s.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fs := &fakeSuggest{}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:   st,
		Auth:    api.NewTokenAuthorizer(adminToken),
		Log:     slog.New(slog.DiscardHandler),
		Suggest: fs,
		Search:  fakeSearch{},
		Events:  events.NewBus(),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, fs
}

// seedProposal writes a submitted proposal with one acquisition (Speed).
func seedProposal(t *testing.T, st store.Store, id string) {
	t.Helper()
	body := `{"acquisitions":[{"mediaType":"movie","tmdbId":100,"name":"Speed","year":1994}]}`
	err := st.CreateProposal(context.Background(), store.Proposal{
		ID: id, JobID: "job-1", Status: "submitted", CreatedBy: "alice", ProposalJSON: body,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestApprove_InLibraryPickBecomesAvailable is the regression test for the smoke
// bug: approval only enqueued acquisitions, so an in-library lineup pick never
// became an `available` title Record and the scheduler could not place it (§8
// "the approved lineup feeds the scheduler"). Approval must create an available
// Record (with the library item id) for each in-library pick.
func TestApprove_InLibraryPickBecomesAvailable(t *testing.T) {
	srv, st, _ := newSuggestServer(t)
	// A proposal whose lineup has one in-library pick (The Matrix) and no acquisitions.
	body := `{"lineup":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix","year":1999,"inLibrary":true,"libraryItemId":"641641"}],"acquisitions":[]}`
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: "p-lib", JobID: "job-lib", Status: "submitted", ProposalJSON: body,
	}); err != nil {
		t.Fatal(err)
	}

	resp := do(t, srv, http.MethodPost, "/v1/suggestions/p-lib/approve", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve → %d, want 200", resp.StatusCode)
	}

	// The in-library pick is now an `available` title the scheduler can resolve.
	avail, _ := st.ListTitlesByState(context.Background(), provision.Available)
	if len(avail) != 1 {
		t.Fatalf("approve created %d available titles, want 1 (the in-library pick)", len(avail))
	}
	if got := string(avail[0].Key); got != "movie:tmdb:603" {
		t.Errorf("available title key = %q, want movie:tmdb:603", got)
	}
	if avail[0].LibraryID != "641641" {
		t.Errorf("available title libraryID = %q, want 641641 (needed to play + resolve duration)", avail[0].LibraryID)
	}
}

func TestSubmit_AnyAuthenticatedUser(t *testing.T) {
	srv, _, fs := newSuggestServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/suggestions", adminToken, `{"description":"90s action"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit → %d, want 200", resp.StatusCode)
	}
	if fs.submits != 1 {
		t.Errorf("submit not invoked: %d", fs.submits)
	}
}

// THE APPROVAL GATE (§19): approve requires admin. A member (anonymous here /
// wrong token) gets 403 — and crucially, no title is enqueued.
func TestApprove_RequiresAdmin_NothingEnqueued(t *testing.T) {
	srv, st, _ := newSuggestServer(t)
	seedProposal(t, st, "p1")

	resp := do(t, srv, http.MethodPost, "/v1/suggestions/p1/approve", "", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member approve → %d, want 403", resp.StatusCode)
	}
	// The approval-gate guarantee: NOTHING unapproved reached /v1/titles.
	wanted, _ := st.ListTitlesByState(context.Background(), "wanted")
	if len(wanted) != 0 {
		t.Fatalf("a denied approval still enqueued %d titles — approval gate breached", len(wanted))
	}
	// The proposal is untouched.
	p, _ := st.GetProposal(context.Background(), "p1")
	if p.Status != "submitted" {
		t.Errorf("proposal status changed on a forbidden approve: %s", p.Status)
	}
}

// Admin approve enqueues the acquisitions as wanted titles (the ONLY path from a
// proposal to /v1/titles) and flips the proposal to approved.
func TestApprove_Admin_EnqueuesAcquisitions(t *testing.T) {
	srv, st, _ := newSuggestServer(t)
	seedProposal(t, st, "p1")

	resp := do(t, srv, http.MethodPost, "/v1/suggestions/p1/approve", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin approve → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Status   string
		Enqueued int
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "approved" || body.Enqueued != 1 {
		t.Errorf("approve body = %+v, want approved/1", body)
	}
	// Speed (tmdb 100) is now a wanted title.
	rec, err := st.GetTitle(context.Background(), "movie:tmdb:100")
	if err != nil {
		t.Fatalf("acquisition not enqueued: %v", err)
	}
	if rec.State != "wanted" {
		t.Errorf("enqueued title state = %s, want wanted", rec.State)
	}
	// The proposal is approved + audited.
	p, _ := st.GetProposal(context.Background(), "p1")
	if p.Status != "approved" {
		t.Errorf("proposal status = %s, want approved", p.Status)
	}
}

// Approve is idempotent-ish: re-approving an already-approved proposal 409s
// (can't double-enqueue).
func TestApprove_AlreadyApproved409(t *testing.T) {
	srv, st, _ := newSuggestServer(t)
	seedProposal(t, st, "p1")
	_ = do(t, srv, http.MethodPost, "/v1/suggestions/p1/approve", adminToken, "")
	resp := do(t, srv, http.MethodPost, "/v1/suggestions/p1/approve", adminToken, "")
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("re-approve → %d, want 409", resp.StatusCode)
	}
}

func TestDeny_RequiresAdmin(t *testing.T) {
	srv, st, _ := newSuggestServer(t)
	seedProposal(t, st, "p1")
	resp := do(t, srv, http.MethodPost, "/v1/suggestions/p1/deny", "", `{"reason":"no"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member deny → %d, want 403", resp.StatusCode)
	}
}

func TestListProposals_ApprovalQueue(t *testing.T) {
	srv, st, _ := newSuggestServer(t)
	seedProposal(t, st, "p1")
	seedProposal(t, st, "p2")
	resp := do(t, srv, http.MethodGet, "/v1/suggestions?status=submitted", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d", resp.StatusCode)
	}
	var body struct {
		Proposals []struct{ ID, Status string }
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Proposals) != 2 {
		t.Errorf("approval queue = %d, want 2", len(body.Proposals))
	}
}

func TestSearch_AnyAuthenticatedUser(t *testing.T) {
	srv, _, _ := newSuggestServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/search?q=matrix", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search → %d", resp.StatusCode)
	}
	var body struct {
		Candidates []struct {
			Name      string
			InLibrary bool
		}
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Candidates) != 1 || body.Candidates[0].Name != "The Matrix" {
		t.Errorf("search results = %+v", body.Candidates)
	}
}

func TestSearch_RequiresQuery(t *testing.T) {
	srv, _, _ := newSuggestServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/search", adminToken, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("search without q → %d, want 400", resp.StatusCode)
	}
}

func TestEvents_RequiresAuth(t *testing.T) {
	srv, _, _ := newSuggestServer(t)
	// Anonymous → 401.
	resp := do(t, srv, http.MethodGet, "/v1/events", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous /v1/events → %d, want 401", resp.StatusCode)
	}
}
