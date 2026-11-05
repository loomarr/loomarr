package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

// TestCreateChannelRefusesIntentRef pins that POST /v1/channels is not a second way to bind a
// channel to an approved proposal: approval is the only path that does that, so a body naming an
// intentRef is rejected and nothing is created or reconciled.
func TestCreateChannelRefusesIntentRef(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, st, chSvc := harness.Server, harness.Store, harness.Channels
	if err := createProposalWithJob(st, context.Background(), store.Proposal{
		ID: "prop-1", JobID: "job-abc", Status: "approved",
		Body: wireProposal(t, `{"intent":{"description":"late-night sci-fi"},"lineup":[{"mediaType":"movie","tmdbId":90000603,"name":"Marigold Protocol","year":1999,"inLibrary":true,"libraryItemId":"12345"}]}`),
	}); err != nil {
		t.Fatal(err)
	}

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"scifi","name":"Late Night Sci-Fi","number":42,"strategy":"shuffle","intentRef":"job-abc"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("create with intentRef -> %d, want 422", resp.StatusCode)
	}
	if _, err := st.GetChannel(context.Background(), "scifi"); err == nil {
		t.Error("a channel was created from an intentRef")
	}
	if chSvc.reconciles != 0 {
		t.Errorf("no reconcile should fire for a rejected create, got %d", chSvc.reconciles)
	}
}

// TestCreateHandMadeChannel confirms a hand-made channel starts with an empty lineup, filled
// later via PATCH, and carries no intent binding.
func TestCreateHandMadeChannel(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, st := harness.Server, harness.Store
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"manual","name":"Manual","number":9,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hand-made create -> %d, want 200", resp.StatusCode)
	}
	ch, err := st.GetChannel(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Lineup) != 0 {
		t.Errorf("hand-made channel lineup = %d, want 0", len(ch.Lineup))
	}
	if ch.IntentRef != "" {
		t.Errorf("hand-made channel intentRef = %q, want empty", ch.IntentRef)
	}
}

// TestCreateSingleSeriesChannel pins the other hand-made shape: a body naming one series seeds a
// one-entry lineup carrying the season range, and a non-series key is refused.
func TestCreateSingleSeriesChannel(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, st := harness.Server, harness.Store
	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"show","name":"Show","number":10,"strategy":"sequential","series":{"key":"series:tvdb:90000655","title":"Cold Relay","seasonMin":2,"seasonMax":3}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("series create -> %d, want 200", resp.StatusCode)
	}
	ch, err := st.GetChannel(context.Background(), "show")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Lineup) != 1 || string(ch.Lineup[0].Key) != "series:tvdb:90000655" ||
		ch.Lineup[0].SeasonMin != 2 || ch.Lineup[0].SeasonMax != 3 || ch.Lineup[0].Title != "Cold Relay" {
		t.Fatalf("series lineup = %+v", ch.Lineup)
	}

	bad := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"movie","name":"Movie","number":11,"strategy":"sequential","series":{"key":"movie:tmdb:90000603"}}`)
	if bad.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("non-series key -> %d, want 422", bad.StatusCode)
	}
}
