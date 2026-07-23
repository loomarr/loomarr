package programmer_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/testkit"
)

// Guide parsing runs against the PINNED capture
// (fixtures/tunarr/guide_channels_response.json, Tunarr 1.3.8) rather than remembered
// field names — the CLAUDE.md rule. It matters more than usual here because the vendored
// OpenAPI does NOT type the guide response, so the only two authorities are Tunarr's own
// zod schemas at the pinned tag and this capture.
//
// It also pins the trap recorded in GUIDE-FINDINGS.md: an earlier pass sampled the
// SINGULAR /api/guide/channels/{id}, whose shape is leaner ({index, lineupItem,
// startTimeMs}) and carries NO title. Asserting a real title here is what proves the
// adapter reads the plural endpoint's nested `program.title`, not that thinner shape.
func TestGuide_ParsesPinnedCapture(t *testing.T) {
	raw := testkit.Fixture(t, "tunarr/guide_channels_response.json")

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	from := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	guide, err := programmer.New(srv.URL, "cfg").Guide(context.Background(), from, from.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("Guide over the pinned capture: %v", err)
	}

	// The plural endpoint — one call for every channel, not one per card.
	if gotPath != "/api/guide/channels" {
		t.Errorf("path = %q, want /api/guide/channels (the singular one has no titles)", gotPath)
	}
	if gotQuery == "" {
		t.Error("guide must be requested for a time window (dateFrom/dateTo)")
	}

	if len(guide) == 0 {
		t.Fatal("pinned capture should yield at least one channel")
	}
	var entries []programmer.GuideEntry
	for _, e := range guide {
		entries = e
		break
	}
	if len(entries) == 0 {
		t.Fatal("pinned capture's channel should have programs")
	}

	first := entries[0]
	// A title is the whole reason for the plural endpoint.
	if first.Title == "" {
		t.Error("entry has no Title — the adapter is reading the title-less singular shape")
	}
	// Real airtimes, not a computed guess.
	if first.StartMs == 0 || first.StopMs <= first.StartMs {
		t.Errorf("entry timing = [%d,%d), want a real start/stop window", first.StartMs, first.StopMs)
	}
	if first.Kind == "" {
		t.Error("entry has no Kind — flex gaps must be distinguishable from programs")
	}
	// identifiers[] carries tmdb, which joins to Loomarr's provisioning key with no
	// second lookup. If Tunarr drops it, the Channels page silently loses that join.
	if first.Kind == "content" && first.TMDBID == "" {
		t.Error("content entry has no TMDBID — the identifiers[] join is gone")
	}
}

// A guide that has not been generated yet is "nothing to show", not an error — the same
// tolerance GetLineup applies to an unprogrammed channel (Phase-0 finding 4).
func TestGuide_UngeneratedGuideIsEmptyNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	guide, err := programmer.New(srv.URL, "cfg").Guide(context.Background(), time.Now(), time.Now())
	if err != nil {
		t.Fatalf("an ungenerated guide must not error: %v", err)
	}
	if len(guide) != 0 {
		t.Errorf("guide = %v, want empty", guide)
	}
}
