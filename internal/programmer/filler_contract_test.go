package programmer_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/testkit"
)

// TestEnsureFillerList_ConsumesPinnedEnumerateFixture wires the captured-but-
// previously-unwired Phase-0 Tunarr filler-lists capture
// (fixtures/tunarr/filler_lists_response.json — the `[]` empty-list shape per that
// dir's FINDINGS.md) through the REAL adapter read path. EnsureFillerList first
// does GET /api/filler-lists and decodes it into []fillerListSummary (filler.go:223)
// for its enumerate-first idempotency check. We serve the pinned capture there
// (like tunarr_test.go serves its fixtures via httptest) and confirm the adapter
// actually consumes those bytes: an empty list + empty desired set is the
// idempotent no-op (found == nil, nothing to detach), so EnsureFillerList must
// return nil having parsed the pinned enumerate response. This is the "parsers
// against fixtures, not remembered field names" rule (CLAUDE.md) for filler-lists —
// if Tunarr changes the enumerate shape, the decode into fillerListSummary breaks
// here.
func TestEnsureFillerList_ConsumesPinnedEnumerateFixture(t *testing.T) {
	raw := testkit.Fixture(t, "tunarr/filler_lists_response.json")

	var hitEnumerate bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/filler-lists" {
			hitEnumerate = true
			_, _ = w.Write(raw)
			return
		}
		// No other endpoint should be reached: an empty desired set with no existing
		// list is a pure no-op after the enumerate read.
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "", "cfg")
	if err := c.EnsureFillerList(context.Background(), "tunarr-ch-1", nil); err != nil {
		t.Fatalf("EnsureFillerList over the pinned empty [] capture must be a no-op, got: %v", err)
	}
	if !hitEnumerate {
		t.Fatal("EnsureFillerList did not read GET /api/filler-lists (the pinned capture was never exercised)")
	}
}

// TestFillerListPrograms_PinnedShapeReadThroughAdapter drives the OTHER real read
// that decodes a filler-lists array: existingProgramIDs does GET
// /api/filler-lists/{id}/programs and decodes into []fillerListProgramEntry
// (filler.go:231). The pinned capture is the same top-level empty-array shape, so
// serving it on the programs sub-path and driving EnsureFillerList past its
// idempotency comparison exercises that decode too. Here we serve a NON-empty
// filler list on the enumerate (a matching list already exists) plus the pinned
// `[]` on its /programs read; because the desired set is empty, EnsureFillerList
// takes the detach path (DELETE the list, clear the channel) — proving the adapter
// read and acted on the enumerate result without error.
func TestFillerListPrograms_PinnedShapeReadThroughAdapter(t *testing.T) {
	raw := testkit.Fixture(t, "tunarr/filler_lists_response.json") // "[]"

	var deleted, clearedChannel bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/filler-lists":
			// An existing list whose stable name matches loomarr:<tunarrID>.
			_, _ = w.Write([]byte(`[{"id":"fl-1","name":"loomarr:ch-x","contentCount":3}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/filler-lists/fl-1/programs":
			// The pinned array shape decoded into []fillerListProgramEntry.
			_, _ = w.Write(raw)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/filler-lists/fl-1":
			deleted = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/channels/ch-x":
			// attachFillerList reads the channel back before clearing fillerCollections.
			_, _ = w.Write([]byte(`{"id":"ch-x","number":1,"name":"n","groupTitle":"g"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/channels/ch-x":
			clearedChannel = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := programmer.New(srv.URL, "", "cfg")
	// Empty desired set + an existing matching list → detach path (DELETE + clear).
	if err := c.EnsureFillerList(context.Background(), "ch-x", nil); err != nil {
		t.Fatalf("EnsureFillerList detach over pinned shapes: %v", err)
	}
	if !deleted {
		t.Error("existing filler list was not cleared when the desired set went empty")
	}
	if !clearedChannel {
		t.Error("channel fillerCollections was not cleared on detach")
	}
}
