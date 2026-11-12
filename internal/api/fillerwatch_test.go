package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/split"
)

func completeSourceCheck(t *testing.T, st records.Store, id string, checkedAt time.Time) {
	t.Helper()
	claimAt, leaseUntil := checkedAt.Add(-time.Minute), checkedAt.Add(29*time.Minute)
	claimed, err := st.ClaimFillerSourceCheck(t.Context(), id, time.Time{}, claimAt, leaseUntil)
	if err != nil || !claimed {
		t.Fatalf("claim source check %q = %v, %v", id, claimed, err)
	}
	if err := st.CompleteFillerSourceCheck(t.Context(), id, leaseUntil, acquisition.SourceCheckCompletion{CheckedAt: checkedAt}); err != nil {
		t.Fatal(err)
	}
}

// The member-visible health projection describes the generation doing the work, not a saved
// layout that is waiting on restart. Otherwise a pending clear could report zero sources while
// this process is still scanning and playing from the applied root.
func TestFillerWatch_CountsAppliedLayoutUntilRestart(t *testing.T) {
	st := openTestStore(t, t.TempDir()+"/watch-applied.db")
	t.Cleanup(func() { _ = st.Close() })
	clearSeededSources(t, st)
	if err := st.UpsertFillerSource(t.Context(),
		records.NewFillerSource("folder", "folder", "", "Drop folder", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	layout, err := cliplibrary.NewLayout("/data/filler", "")
	if err != nil {
		t.Fatal(err)
	}
	live := mapSettings{"filler.dir": "/data/filler", "filler.home_country": "US"}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         testAuthorizer{},
		Log:          slog.New(slog.DiscardHandler),
		Filler:       api.FillerOptions{Layout: layout},
		LiveSettings: live,
	})

	live["filler.dir"] = "" // persisted desired clear; the current generation still has one applied source
	req := httptest.NewRequest(http.MethodGet, "/v1/filler/watch", nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("watch → %d, want 200", rec.Code)
	}
	var body watchBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.SourcesTotal != 1 || body.SourcesOn != 1 {
		t.Errorf("sources = %d of %d after desired clear, want applied 1 of 1 until restart", body.SourcesOn, body.SourcesTotal)
	}
	if body.SourcesReady != 1 {
		t.Errorf("ready sources = %d after desired clear, want 1", body.SourcesReady)
	}
}

// GET /v1/filler/watch — the Filler header's live status (§10 V38c).
//
// ⚠ These tests exist because the first implementation derived the verdict IN THE BROWSER. That
// was wrong twice: the rule is domain logic (testable here against a real store, not against a
// hand-built fixture array), and `/v1/filler/sources` is admin-only — so a member's indicator sat
// permanently grey, saying "nothing set up" about a working install.

type watchBody struct {
	Health       string `json:"health"`
	SourcesOn    int    `json:"sourcesOn"`
	SourcesReady int    `json:"sourcesReady"`
	SourcesTotal int    `json:"sourcesTotal"`
	Clips        int    `json:"clips"`
	Preparing    int    `json:"preparing"`
	NeedsYou     int    `json:"needsYou"`
	LastScanAt   string `json:"lastScanAt"`
	AutoFetch    *struct {
		Enabled      bool
		StoppedBy    string
		CatalogClips int
		MaxCatalog   int
	} `json:"autoFetch"`
}

func newFillerWatchServer(t *testing.T) (*httptest.Server, records.Store, *fakeFiller) {
	t.Helper()
	return newFillerServerWithConfig(t, nil, func(key string) string {
		if key == "filler.home_country" {
			return "US"
		}
		return ""
	})
}

func TestFillerWatch_ReportsTheLiveFetchCeiling(t *testing.T) {
	srv, _, ff := newFillerWatchServer(t)
	ff.fetchStatus = acquisition.FetchStatus{
		Enabled: true, StoppedBy: "catalog",
		CatalogClips: 2000, MaxCatalog: 2000,
	}

	body, code := getWatch(t, srv.URL, memberToken)
	if code != http.StatusOK {
		t.Fatalf("watch → %d, want 200", code)
	}
	if body.AutoFetch == nil || body.AutoFetch.StoppedBy != "catalog" || body.AutoFetch.CatalogClips != 2000 || body.AutoFetch.MaxCatalog != 2000 {
		t.Fatalf("autoFetch = %+v, want live catalog ceiling 2000/2000", body.AutoFetch)
	}
}

// seedHeldClip is a clip that arrived but has not been terminally admitted.
func seedHeldClip(t *testing.T, st records.Store, id string) {
	t.Helper()
	seedClip(t, st, id, clips.Commercial, 1992, clips.Kids, "toys")
	// HoldClips is the ordinary one-way writer; terminal applied admission is the atomic release.
	if _, err := st.HoldClips(t.Context(), []string{id}, time.Now().UTC()); err != nil {
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
	srv, st, _ := newFillerWatchServer(t)
	seedClip(t, st, "a.mp4", clips.Commercial, 1992, clips.Kids, "toys")

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
	srv, _, _ := newFillerWatchServer(t)

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
	srv, st, _ := newFillerWatchServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", clips.Commercial, 1992, clips.Kids, "toys")

	src := records.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())
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
	srv, st, _ := newFillerWatchServer(t)
	if err := st.UpsertFillerSource(t.Context(),
		records.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "attention" {
		t.Errorf("health = %q, want attention — sources are on but the catalog is empty", body.Health)
	}
}

// The healthy path, and the counts the header renders.
func TestFillerWatch_ReportsCountsAndHealth(t *testing.T) {
	srv, st, _ := newFillerWatchServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", clips.Commercial, 1992, clips.Kids, "toys")
	seedClip(t, st, "b.mp4", clips.Commercial, 1994, clips.Kids, "cereal")

	for _, id := range []string{"classic", "retro"} {
		if err := st.UpsertFillerSource(ctx,
			records.NewFillerSource(id, "archive", id, id, time.Now().UTC())); err != nil {
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
	if body.SourcesReady != 1 {
		t.Errorf("ready sources = %d, want 1", body.SourcesReady)
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
	srv, st, _ := newFillerWatchServer(t)
	if err := st.UpsertFillerSource(t.Context(),
		records.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		seedHeldClip(t, st, id)
		if err := st.UpsertClipPipeline(t.Context(), pipelinestate.ClipPipeline{
			ClipHash: id, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionRunning, UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
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
	if body.Preparing != 3 {
		t.Errorf("preparing = %d, want 3 — an install preparing clips must say so, not report nothing", body.Preparing)
	}
	if body.NeedsYou != 0 {
		t.Errorf("needsYou = %d, want 0 — clips being prepared are machine work, not a choice", body.NeedsYou)
	}
}

// #2733: the header's "N need you" and "N getting ready" are Incoming's own totals. A held clip
// that the pipeline is preparing is not a choice, and only a split waiting for an answer is; the
// header used to count every held clip as "need you" while Incoming listed none of them.
func TestFillerWatch_CountsAreIncomingsTotals(t *testing.T) {
	srv, st, _ := newFillerWatchServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	for i, hash := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		seedHeldClip(t, st, hash)
		if err := st.UpsertClipPipeline(t.Context(), pipelinestate.ClipPipeline{
			ClipHash: hash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionRunning, UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	putClip(t, st, clips.Clip{Hash: "reel", Path: "reel.mp4", Name: "Reel", Held: true, IsComposite: true})
	if err := st.UpsertSplitProposal(t.Context(), filler.SplitProposal{
		ID: "split-1", ClipHash: "reel", CreatedAt: now,
		Segments: []split.Segment{{Index: 0, StartMs: 0, EndMs: 30_000, Name: "first clip"}},
	}); err != nil {
		t.Fatal(err)
	}

	watch, _ := getWatch(t, srv.URL, adminToken)
	_, incoming := readIncoming(t, srv.URL, "/v1/filler/incoming", adminToken)
	if watch.NeedsYou != incoming.NeedsHelp.Total || watch.NeedsYou != 1 {
		t.Errorf("needsYou = %d, Incoming needs help = %d, want both 1", watch.NeedsYou, incoming.NeedsHelp.Total)
	}
	if watch.Preparing != incoming.Preparing.Total || watch.Preparing != 3 {
		t.Errorf("preparing = %d, Incoming preparing = %d, want both 3", watch.Preparing, incoming.Preparing.Total)
	}
}

// The mirror, so the healthy-on-held rule above cannot be satisfied by never flagging anything:
// sources on, and genuinely NOTHING anywhere, is still attention.
func TestFillerWatch_NoClipsAtAllIsStillAttention(t *testing.T) {
	srv, st, _ := newFillerWatchServer(t)
	if err := st.UpsertFillerSource(t.Context(),
		records.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "attention" {
		t.Errorf("health = %q, want attention — nothing catalogued AND nothing held", body.Health)
	}
	if body.Preparing != 0 || body.NeedsYou != 0 {
		t.Errorf("preparing = %d, needsYou = %d, want 0 and 0", body.Preparing, body.NeedsYou)
	}
}

// ⚠ A source that has NEVER been fetched must not read as stale. Most sources are SCANNED rather
// than fetched, so an absent timestamp is the ordinary state — treating it as stale would light
// every drop-folder install amber forever, which is how an operator learns to ignore the dot.
func TestFillerWatch_NeverFetchedIsNotStale(t *testing.T) {
	srv, st, _ := newFillerWatchServer(t)
	seedClip(t, st, "a.mp4", clips.Commercial, 1992, clips.Kids, "toys")
	if err := st.UpsertFillerSource(t.Context(),
		records.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
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
	srv, st, _ := newFillerWatchServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", clips.Commercial, 1992, clips.Kids, "toys")
	if err := st.UpsertFillerSource(ctx,
		records.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	completeSourceCheck(t, st, "classic", time.Now().UTC().Add(-5*24*time.Hour))

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
	srv, st, _ := newFillerWatchServer(t)
	ctx := t.Context()
	seedClip(t, st, "a.mp4", clips.Commercial, 1992, clips.Kids, "toys")
	for _, id := range []string{"stale", "fresh"} {
		if err := st.UpsertFillerSource(ctx,
			records.NewFillerSource(id, "archive", id, id, time.Now().UTC())); err != nil {
			t.Fatal(err)
		}
	}
	completeSourceCheck(t, st, "stale", time.Now().UTC().Add(-30*24*time.Hour))
	completeSourceCheck(t, st, "fresh", time.Now().UTC().Add(-time.Minute))

	body, _ := getWatch(t, srv.URL, adminToken)
	if body.Health != "healthy" {
		t.Errorf("health = %q, want healthy — one source ran a minute ago", body.Health)
	}
}
