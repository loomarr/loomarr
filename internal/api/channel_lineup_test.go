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

// seedApprovedProposalWithAcquisition writes an approved proposal carrying one
// in-library pick AND one acquisition (a title not yet owned). It mirrors what
// the suggester emits when an intent needs content acquired to be complete.
func seedApprovedProposalWithAcquisition(t *testing.T, st store.Store, jobID, propID string) {
	t.Helper()
	// lineup: movie:tmdb:603 (The Matrix, owned). acquisitions: movie:tmdb:604
	// (The Matrix Reloaded, missing — must still become a lineup entry so it's
	// placed once it lands).
	proposalJSON := `{
		"intent": {"description":"the matrix marathon"},
		"lineup": [
			{"mediaType":"movie","tmdbId":603,"name":"The Matrix","year":1999,"inLibrary":true,"libraryItemId":"12345"}
		],
		"acquisitions": [
			{"mediaType":"movie","tmdbId":604,"name":"The Matrix Reloaded","year":2003,"inLibrary":false}
		]
	}`
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: propID, JobID: jobID, Status: "approved", ProposalJSON: proposalJSON,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestCreateChannelBindsAcquisitionsAsPendingEntries is the regression test for
// the #9 SEVERE seam: acquisitions never entered ch.Lineup, so once an acquired
// title landed `available` it had NO entry to fill and was permanently
// unschedulable (the sweep re-derives desired slots from ch.Lineup). The fix
// carries every approved pick — in-library AND acquisitions — into ch.Lineup;
// an acquisition starts as a pending slot and swaps to a program in place as it
// lands. This asserts the acquisition key is present in the stored lineup, which
// the old `continue`-on-`!InLibrary` code dropped (would yield 1 entry, not 2).
func TestCreateChannelBindsAcquisitionsAsPendingEntries(t *testing.T) {
	srv, st, _, _ := newServerWithScheduler(t)
	seedApprovedProposalWithAcquisition(t, st, "job-mtx", "prop-mtx")

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"matrix","name":"Matrix Marathon","number":43,"strategy":"sequential","intentRef":"job-mtx"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create → %d, want 200", resp.StatusCode)
	}

	ch, err := st.GetChannel(context.Background(), "matrix")
	if err != nil {
		t.Fatal(err)
	}
	// BOTH the in-library pick and the acquisition must be lineup entries — the
	// acquisition included, so backfill can place it once it lands.
	if len(ch.Lineup) != 2 {
		t.Fatalf("channel lineup = %d entries, want 2 (in-library + acquisition)", len(ch.Lineup))
	}
	got := map[string]bool{}
	for _, e := range ch.Lineup {
		got[string(e.Key)] = true
	}
	if !got["movie:tmdb:603"] {
		t.Error("in-library pick movie:tmdb:603 missing from lineup")
	}
	if !got["movie:tmdb:604"] {
		t.Error("acquisition movie:tmdb:604 missing from lineup — #9 regression (acquisitions never placed)")
	}
	// Ordering contract: in-library picks precede acquisitions.
	if string(ch.Lineup[0].Key) != "movie:tmdb:603" {
		t.Errorf("lineup[0] = %q, want the in-library pick first (movie:tmdb:603)", ch.Lineup[0].Key)
	}
}
