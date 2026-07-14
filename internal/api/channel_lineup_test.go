package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
)

// seedApprovedProposal writes an approved proposal (one in-library movie) for a
// given job id, mirroring what the suggester + approval gate persist.
func seedApprovedProposal(t *testing.T, st store.Store, jobID, propID string) {
	t.Helper()
	// A minimal grounded proposal: one in-library title (movie:tmdb:603 = The Matrix).
	proposalJSON := `{
		"intent": {"description":"late-night sci-fi"},
		"lineup": [
			{"mediaType":"movie","tmdbId":603,"name":"The Matrix","year":1999,"inLibrary":true,"libraryItemId":"12345"}
		],
		"acquisitions": null
	}`
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: propID, JobID: jobID, Status: "approved", ProposalJSON: proposalJSON,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestCreateChannelBindsApprovedLineup is the regression test for the seam bug
// the live smoke found: createChannel ignored intentRef, so a channel built from
// an approved proposal came up with an EMPTY lineup (0 programs). It must copy the
// approved proposal's in-library lineup into the channel (§7/§9).
func TestCreateChannelBindsApprovedLineup(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedApprovedProposal(t, st, "job-abc", "prop-1")

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"scifi","name":"Late Night Sci-Fi","number":42,"strategy":"shuffle","intentRef":"job-abc"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create → %d, want 200", resp.StatusCode)
	}

	// The channel's stored lineup must carry the approved proposal's in-library pick
	// — NOT be empty. This is the assertion that would have caught the bug.
	ch, err := st.GetChannel(context.Background(), "scifi")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Lineup) != 1 {
		t.Fatalf("channel lineup = %d entries, want 1 (the approved in-library title)", len(ch.Lineup))
	}
	if got := string(ch.Lineup[0].Key); got != "movie:tmdb:603" {
		t.Errorf("lineup key = %q, want canonical movie:tmdb:603", got)
	}
	if ch.Lineup[0].Title != "The Matrix" {
		t.Errorf("lineup title = %q, want The Matrix", ch.Lineup[0].Title)
	}
}

// TestCreateChannelRejectsUnapprovedIntent enforces the approval gate at the
// channel seam: a channel cannot be built from an intent whose proposal was never
// approved (prime directive #3 — unapproved content must not reach a live channel).
func TestCreateChannelRejectsUnapprovedIntent(t *testing.T) {
	srv, st, chSvc, _ := newServerWithScheduler(t)
	// A proposal exists but is still "submitted" (not approved).
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: "prop-x", JobID: "job-unapproved", Status: "submitted",
		ProposalJSON: `{"intent":{"description":"x"},"lineup":[]}`,
	}); err != nil {
		t.Fatal(err)
	}

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c","name":"n","number":7,"strategy":"sequential","intentRef":"job-unapproved"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("create from unapproved intent → %d, want 422", resp.StatusCode)
	}
	// And nothing was created or reconciled.
	if _, err := st.GetChannel(context.Background(), "c"); err == nil {
		t.Error("channel should NOT have been created from an unapproved intent")
	}
	if chSvc.reconciles != 0 {
		t.Errorf("no reconcile should fire for a rejected create, got %d", chSvc.reconciles)
	}
}

// TestCreateHandMadeChannelNoIntent confirms a channel with no intentRef still
// works (hand-made): empty lineup is legitimate, filled later via PUT.
func TestCreateHandMadeChannelNoIntent(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"manual","name":"Manual","number":9,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hand-made create → %d, want 200", resp.StatusCode)
	}
	ch, err := st.GetChannel(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Lineup) != 0 {
		t.Errorf("hand-made channel lineup = %d, want 0 (filled later via PUT)", len(ch.Lineup))
	}
}
