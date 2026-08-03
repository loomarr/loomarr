package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// GET /v1/filler/watch — the Filler header's live status (§10 V38c).
//
// ⚠ These tests exist because the first implementation derived the verdict IN THE BROWSER. That
// was wrong twice: the rule is domain logic (testable here against a real store, not against a
// hand-built fixture array), and `/v1/filler/sources` is admin-only — so a member's indicator sat
// permanently grey, saying "nothing set up" about a working install.

type watchBody struct {
	Health       string `json:"health"`
	SourcesOn    int    `json:"sourcesOn"`
	SourcesTotal int    `json:"sourcesTotal"`
	Clips        int    `json:"clips"`
	Held         int    `json:"held"`
	LastScanAt   string `json:"lastScanAt"`
}

// seedHeldClip is a clip that ARRIVED but has not been filed — the state auto-fetch leaves
// everything in until a human reviews it (§10 V38).
func seedHeldClip(t *testing.T, st store.Store, id string) {
	t.Helper()
	seedClip(t, st, id, filler.Commercial, 1992, filler.Kids, "toys")
	// SetClipsHeld is the only writer of `held`, deliberately — see internal/store/clips.go.
	if _, err := st.SetClipsHeld(t.Context(), []string{id}, true, false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func getWatch(t *testing.T, srvURL, token string) (watchBody, int) {
	t.Helper()
	res := sourceReq(t, http.MethodGet, srvURL+"/v1/filler/watch", "", token)
	var body watchBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}
	return body, res.StatusCode
}

// ⚠ THE reason this route exists. A member can see whether filler is working; they cannot see
// WHERE it comes from. The sources listing stays admin-only because it names filesystem paths and
// library targets — this carries counts and a verdict, and nothing else.
func TestFillerWatch_IsMemberReadable(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "a.mp4", filler.Commercial, 1992, filler.Kids, "toys")

	body, code := getWatch(t, srv.URL, memberToken)
	if code != http.StatusOK {
		t.Fatalf("member got %d, want 200 — a member's pill would be permanently grey", code)
	}
	if body.Clips != 1 {
		t.Errorf("clips = %d, want 1", body.Clips)
	}

	// And it discloses no infrastructure: the response carries no paths or targets at all.
	res := sourceReq(t, http.MethodGet, srv.URL+"/v1/filler/watch", "", memberToken)
	var raw map[string]any
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"target", "uri", "path", "detail"} {
		if _, ok := raw[leaked]; ok {
			t.Errorf("response carries %q — this route is member-visible and must name no infrastructure", leaked)
		}
	}
}

// A fresh install is UNCONFIGURED, not broken. An amber warning on first boot reads as a fault
// the operator caused, which is the opposite of the truth: there is simply work still to do.
func TestFillerWatch_FreshInstallIsUnconfiguredNotBroken(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	body, code := getWatch(t, srv.URL, adminToken)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body.Health != "unconfigured" {
		t.Errorf("health = %q, want unconfigured on an install with nothing set up", body.Health)
	}
}

// THE failure the hardcoded green dot hid: every source switched off, nothing scanning, and a
// reassuring pulse claiming otherwise.
func TestFillerWatch_AllSourcesOffAsksForAttention(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", filler.Commercial, 1992, filler.Kids, "toys")

	src := store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())
	if err := st.UpsertFillerSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFillerSourceEnabled(ctx, "classic", false); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "attention" {
		t.Errorf("health = %q, want attention — every source is switched off and nothing is scanning", body.Health)
	}
	if body.SourcesOn != 0 {
		t.Errorf("sourcesOn = %d, want 0", body.SourcesOn)
	}
}

// Sources on, catalog empty — usually an empty folder or a mount that did not come up. Worth
// flagging on day one rather than after someone notices a channel playing silence.
func TestFillerWatch_OnButEmptyAsksForAttention(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertFillerSource(t.Context(),
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "attention" {
		t.Errorf("health = %q, want attention — sources are on but the catalog is empty", body.Health)
	}
}

// The healthy path, and the counts the header renders.
func TestFillerWatch_ReportsCountsAndHealth(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", filler.Commercial, 1992, filler.Kids, "toys")
	seedClip(t, st, "b.mp4", filler.Commercial, 1994, filler.Kids, "cereal")

	for _, id := range []string{"classic", "retro"} {
		if err := st.UpsertFillerSource(ctx,
			store.NewFillerSource(id, "archive", id, id, time.Now().UTC())); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetFillerSourceEnabled(ctx, "retro", false); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "healthy" {
		t.Errorf("health = %q, want healthy", body.Health)
	}
	if body.SourcesOn != 1 || body.SourcesTotal != 2 {
		t.Errorf("sources = %d of %d, want 1 of 2", body.SourcesOn, body.SourcesTotal)
	}
	if body.Clips != 2 {
		t.Errorf("clips = %d, want 2", body.Clips)
	}
}

// ⚠ **A HELD clip is not a missing clip.** Auto-fetch holds everything it downloads for review, so
// the very first successful fetch leaves the CATALOG at zero and the review queue full. Counting
// only the catalog made that moment report `attention` and the header read "0 clips" — the
// fetcher's own success rendered as a failure, and indistinguishable from a fetch that never ran.
//
// Found live: an install that had just pulled 12 clips showed "5 of 5 sources on · 0 clips".
func TestFillerWatch_HeldClipsAreCountedSeparatelyNotAsNothing(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertFillerSource(t.Context(),
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		seedHeldClip(t, st, id)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "healthy" {
		t.Errorf("health = %q, want healthy — three clips just arrived and are awaiting review, "+
			"which is the fetcher WORKING", body.Health)
	}
	// Separate numbers, not a sum: the catalog really is empty, and the header must be able to say
	// "0 clips · 3 waiting" rather than overstate what a channel can actually play.
	if body.Clips != 0 {
		t.Errorf("clips = %d, want 0 — nothing has been filed into the catalog yet", body.Clips)
	}
	if body.Held != 3 {
		t.Errorf("held = %d, want 3 — an install holding clips must say so, not report nothing", body.Held)
	}
}

// The mirror, so the healthy-on-held rule above cannot be satisfied by never flagging anything:
// sources on, and genuinely NOTHING anywhere, is still attention.
func TestFillerWatch_NoClipsAtAllIsStillAttention(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	if err := st.UpsertFillerSource(t.Context(),
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "attention" {
		t.Errorf("health = %q, want attention — nothing catalogued AND nothing held", body.Health)
	}
	if body.Held != 0 {
		t.Errorf("held = %d, want 0", body.Held)
	}
}

// ⚠ A source that has NEVER been fetched must not read as stale. Most sources are SCANNED rather
// than fetched, so an absent timestamp is the ordinary state — treating it as stale would light
// every drop-folder install amber forever, which is how an operator learns to ignore the dot.
func TestFillerWatch_NeverFetchedIsNotStale(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	seedClip(t, st, "a.mp4", filler.Commercial, 1992, filler.Kids, "toys")
	if err := st.UpsertFillerSource(t.Context(),
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "healthy" {
		t.Errorf("health = %q, want healthy — a source with no fetch time is normal, not stale", body.Health)
	}
	if body.LastScanAt != "" {
		t.Errorf("lastScanAt = %q, want absent so the client omits the clause entirely", body.LastScanAt)
	}
}

// Everything that reports a fetch time went quiet days ago.
func TestFillerWatch_LongSilenceAsksForAttention(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", filler.Commercial, 1992, filler.Kids, "toys")
	if err := st.UpsertFillerSource(ctx,
		store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkFillerSourceFetched(ctx, "classic", time.Now().UTC().Add(-5*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "attention" {
		t.Errorf("health = %q, want attention — nothing has arrived in five days", body.Health)
	}
	if body.LastScanAt == "" {
		t.Error("lastScanAt is absent, but this source reported a fetch")
	}
}

// A recent fetch on ONE source is enough. A stale archive collection beside a folder that ran
// this morning is not a problem worth a warning.
func TestFillerWatch_OneCurrentSourceKeepsItHealthy(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", filler.Commercial, 1992, filler.Kids, "toys")
	for _, id := range []string{"stale", "fresh"} {
		if err := st.UpsertFillerSource(ctx,
			store.NewFillerSource(id, "archive", id, id, time.Now().UTC())); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.MarkFillerSourceFetched(ctx, "stale", time.Now().UTC().Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkFillerSourceFetched(ctx, "fresh", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "healthy" {
		t.Errorf("health = %q, want healthy — one source ran a minute ago", body.Health)
	}
}
