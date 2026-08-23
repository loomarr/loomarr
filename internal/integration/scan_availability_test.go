package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

// TestScanAvailability_NoWebhook is the retirement safety proof (design §4, build plan): a
// `requested` title reaches `available` purely by the library-scan job observing it in the
// media server — NO inbound Sonarr/Radarr webhook involved. This is the gate that must be
// green before the webhook subsystem is deleted; it proves poll-based availability works
// end-to-end through the real composition root (jobs API → scheduler → LibraryScan → store).
func TestScanAvailability_NoWebhook(t *testing.T) {
	h := newHarness(t) // connections on: library.url/token point at h.ms
	admin := h.asAdmin()

	// A title Loomarr requested and is waiting on — nothing has confirmed it yet.
	key := provision.Key("movie:tmdb:603")
	if err := h.store.UpsertTitle(context.Background(), provision.Record{
		Key: key, State: provision.Requested, Deadline: time.Now().Add(24 * time.Hour),
		Title: provision.Title{MediaType: provision.Movie, TMDBID: 603},
	}); err != nil {
		t.Fatal(err)
	}

	// The media server now reports it present (an import landed) — discoverable ONLY via the
	// bulk scan, never via a webhook to Loomarr.
	h.ms.SetSearchItems(
		testkit.SearchStub{LibraryItemID: "lib-603", Name: "The Matrix", Type: "Movie", TMDBID: 603},
	)

	// Run the scan on demand through the admin jobs API — the same path the scheduler drives.
	// Trigger is async (202 now; the heartbeat runs the job on its own goroutine), exactly as
	// run-now behaves for the FE, so we wait for the effect rather than asserting inline.
	if code := h.status(http.MethodPost, "/v1/jobs/library-scan/run", "", admin); code != http.StatusAccepted {
		t.Fatalf("run library-scan → %d, want 202", code)
	}

	rec := awaitAvailable(t, h, key)
	if rec.State != provision.Available {
		t.Errorf("state = %s, want available (confirmed by scan, no webhook)", rec.State)
	}
	if rec.LibraryID != "lib-603" {
		t.Errorf("LibraryID = %q, want lib-603", rec.LibraryID)
	}
}

// awaitAvailable polls the store until the title is terminal or a short deadline passes — the
// scan job runs asynchronously off the heartbeat after Trigger.
func awaitAvailable(t *testing.T, h *harness, key provision.Key) provision.Record {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		rec, err := h.store.GetTitle(context.Background(), key)
		if err != nil {
			t.Fatalf("GetTitle: %v", err)
		}
		if rec.State.Terminal() || time.Now().After(deadline) {
			return rec
		}
		time.Sleep(20 * time.Millisecond)
	}
}
