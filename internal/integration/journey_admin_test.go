package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit"
)

// TestJourney_NewAdmin drives the entire first-run admin experience through the
// real composition root (app.Build) exactly as the FE will: bootstrap the
// owner, log in locally, read settings + feature gates, test a connection, pick a
// model, then run an intent → approve → channel. Every step protects a seam that
// no other test exercises end to end (the picker's real Prober, the local-Argon2id
// login chain, the /setup/test probe, and the whole api.Options wiring driven over
// HTTP). Grounding depth is covered by TestPipeline_KidsChannel_EndToEnd; here the
// point is that the composition is wired with no surprises for the frontend.
func TestJourney_NewAdmin(t *testing.T) {
	// The scripted suggester: search the catalog by term (grounds to the real library
	// results), then pick the four surfaced ids + a kids policy.
	llm := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "cartoon"}),
		testkit.FinalResponse(`{
			"rationale":"90s Saturday morning cartoons",
			"picks":[
				{"mediaType":"movie","tmdbId":5001,"name":"Sunny Toon Hour"},
				{"mediaType":"movie","tmdbId":5002,"name":"Robo Rangers"},
				{"mediaType":"movie","tmdbId":5003,"name":"Critter Club"},
				{"mediaType":"movie","tmdbId":5004,"name":"Midnight Mayhem Toons"}
			],
			"policy":{"audience":{"ceiling":"TV-Y7"},"era":{"from":1990,"to":1999},"genres":{"include":["Animation"]},"ordering":"syndication"}
		}`),
	)
	h := newHarness(t, withLLM(llm), withSeerr(), withTunarrPlayout())
	h.ms.SetSearchItems(
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-1", Name: "Sunny Toon Hour", Type: "Movie", Year: 1992, TMDBID: 5001, Genres: []string{"Animation"}, OfficialRating: "TV-Y7", RunTimeTicks: hourTicks},
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-2", Name: "Robo Rangers", Type: "Movie", Year: 1993, TMDBID: 5002, Genres: []string{"Animation"}, OfficialRating: "TV-Y7", RunTimeTicks: hourTicks},
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-3", Name: "Critter Club", Type: "Movie", Year: 1994, TMDBID: 5003, Genres: []string{"Animation"}, OfficialRating: "TV-Y", RunTimeTicks: hourTicks},
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-4", Name: "Midnight Mayhem Toons", Type: "Movie", Year: 1994, TMDBID: 5004, Genres: []string{"Animation"}, OfficialRating: "TV-MA", RunTimeTicks: hourTicks},
	)
	seedFillerClips(t, h.store)

	// A1: BOOTSTRAP the owning admin — and a second attempt 409s (bootstrap is
	// self-gated on "no admin exists yet", §11).
	if code := h.bootstrap("owner", "owner-pass"); code != http.StatusOK {
		t.Fatalf("bootstrap → %d, want 200", code)
	}
	if code := h.bootstrap("interloper", "nope"); code != http.StatusConflict {
		t.Fatalf("second bootstrap → %d, want 409", code)
	}

	// A2: LOCAL LOGIN (the Argon2id path — never exercised via API before) → session.
	admin := h.login("owner", "owner-pass")
	var me struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	h.getJSON("/v1/auth/me", admin, &me)
	if me.Name != "owner" || me.Role != "admin" {
		t.Fatalf("/v1/auth/me = %+v, want owner/admin", me)
	}

	// A3: SETTINGS + feature gates. Seerr + LLM + TMDB are configured, so acquisition
	// and suggestions read on (the wizard's green checks come from this map).
	feats := h.features(admin)
	if !feats["acquisition"] || !feats["suggestions"] {
		t.Fatalf("features = %+v, want acquisition+suggestions on", feats)
	}

	// A4: CONNECTION TESTS — the real probes hit the doubles: media_server (ListUsers),
	// tunarr (GetChannel reachability), requester (Seerr.Reachable). All three are the
	// wizard's "Test connection" buttons; all must report ok with the config seeded.
	for _, check := range []string{"media_server", "tunarr", "requester"} {
		var testOut struct {
			OK   bool   `json:"ok"`
			Hint string `json:"hint"`
		}
		resp := h.do(http.MethodPost, "/v1/setup/test", `{"check":"`+check+`"}`, admin)
		decodeBody(t, resp, &testOut)
		if !testOut.OK {
			t.Fatalf("%s connection test failed: %q", check, testOut.Hint)
		}
	}

	// A5: MODEL PICKER probe — the real systemLLMService + Prober over the Ollama
	// double: reachable, provider ollama, version from the double.
	var sys struct {
		Provider      string `json:"provider"`
		Reachable     bool   `json:"reachable"`
		OllamaVersion string `json:"ollamaVersion"`
	}
	h.getJSON("/v1/system/llm", admin, &sys)
	if !sys.Reachable || sys.Provider != "ollama" || sys.OllamaVersion != "0.13.5" {
		t.Fatalf("system/llm = %+v, want reachable ollama 0.13.5", sys)
	}

	// A6: SELECT a model — must be pulled first (409), then succeeds once pulled.
	if code := h.status(http.MethodPost, "/v1/system/llm/select", `{"provider":"ollama","model":"qwen3:8b"}`, admin); code != http.StatusConflict {
		t.Fatalf("select un-pulled model → %d, want 409", code)
	}
	h.ollama.SetPulled("qwen3:8b")
	if code := h.status(http.MethodPost, "/v1/system/llm/select", `{"provider":"ollama","model":"qwen3:8b"}`, admin); code != http.StatusOK {
		t.Fatalf("select pulled model → %d, want 200", code)
	}

	// A7: PULL a model — a background job streaming over SSE; the double records the hit.
	var pull struct {
		JobID string `json:"jobId"`
	}
	pullResp := h.do(http.MethodPost, "/v1/system/llm/pull", `{"model":"llama3.1:8b"}`, admin)
	decodeBody(t, pullResp, &pull)
	if pull.JobID == "" {
		t.Fatal("pull returned no jobId")
	}
	waitFor(t, func() bool { return h.ollama.PullHits() >= 1 }, "ollama /api/pull was never called")

	// A8: SUBMIT an intent → the real worker pool runs the real grounded suggester.
	var submitted struct {
		JobID string `json:"jobId"`
	}
	submitResp := h.do(http.MethodPost, "/v1/proposals", `{"description":"90s Saturday morning cartoons for kids"}`, admin)
	decodeBody(t, submitResp, &submitted)
	if submitted.JobID == "" {
		t.Fatal("submit returned no jobId")
	}
	propID, prop := h.awaitProposal(admin, submitted.JobID)
	// Four cartoons were grounded; the TV-MA one is REFUSED against the proposal's own TV-Y7
	// ceiling (#259), so the admin is asked to approve three. The refusal is visible on the
	// card rather than the title silently vanishing between the model and the approval screen.
	if len(prop.Lineup) != 3 {
		t.Fatalf("expected 3 airable in-library picks, got %d", len(prop.Lineup))
	}
	if len(prop.Refused) != 1 || prop.Refused[0].Reason != "over_ceiling" {
		t.Fatalf("expected the TV-MA toon refused as over_ceiling, got %+v", prop.Refused)
	}
	if prop.Policy.Audience.Ceiling != "TV-Y7" {
		t.Fatalf("policy ceiling = %q, want TV-Y7", prop.Policy.Audience.Ceiling)
	}

	// A9: APPROVE (the real gate) → titles and the local channel commit together.
	var approved struct {
		ChannelID string `json:"channelId"`
	}
	decodeBody(t, h.do(http.MethodPost, "/v1/proposals/"+propID+"/approve", "", admin), &approved)
	if approved.ChannelID == "" {
		t.Fatal("approve returned no channel id")
	}
	for _, key := range []string{"movie:tmdb:5001", "movie:tmdb:5002", "movie:tmdb:5003"} {
		if rec, err := h.store.GetTitle(context.Background(), provision.Key(key)); err != nil || rec.State != "available" {
			t.Fatalf("approve should mark %s available, got %v/%v", key, rec.State, err)
		}
	}

	// A10: The same approval reconciles through the real engine to the injected Tunarr,
	// ENFORCING the policy (the TV-MA toon is excluded).
	ch, _ := h.store.GetChannel(context.Background(), approved.ChannelID)
	if ch.TunarrID == "" {
		t.Fatal("server-assigned TunarrID not persisted")
	}
	lineup, err := h.tun.GetLineup(context.Background(), ch.TunarrID)
	if err != nil {
		t.Fatal(err)
	}
	programItems, programs := map[string]bool{}, 0
	for _, s := range lineup {
		if s.Kind == schedule.SlotProgram {
			programs++
			programItems[s.LibraryItemID] = true
		}
	}
	if programItems["lib-4"] {
		t.Fatal("TV-MA toon reached a TV-Y7 channel — audience enforcement failed end-to-end")
	}
	if programs != 3 {
		t.Errorf("expected 3 program slots (the kids toons), got %d", programs)
	}

	// A11: IDEMPOTENCY — a second reconcile makes no new Tunarr writes.
	pushesBefore := h.tun.Pushes
	if code := h.status(http.MethodPost, "/v1/channels/"+approved.ChannelID+"/reconcile", "", admin); code != http.StatusOK {
		t.Fatalf("reconcile → %d, want 200", code)
	}
	if h.tun.Pushes != pushesBefore {
		t.Errorf("second reconcile re-pushed: %d → %d", pushesBefore, h.tun.Pushes)
	}
}

// A holiday request travels through the public Proposal API and durable worker,
// uses TMDB thematic keywords rather than title matching, and stops in submitted
// review. Discovery may suggest a new title; it must never turn that into approval.
func TestJourney_HolidayKeywordProposalIncludesOutsideLibraryDiscovery(t *testing.T) {
	llm := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"keywords": []any{"Christmas"}, "media_type": "movie",
		}),
		testkit.FinalResponse(`{
			"channelName":"Snow Day Cinema",
			"picks":[{"mediaType":"movie","tmdbId":2401,"name":"Snowbound Reunion","confidence":0.91}],
			"policy":{"seasonal":{"mode":"exclusive"}}
		}`),
	)
	h := newHarness(t, withLLM(llm))
	h.tmdb.AddKeywordMovie(2401, "Snowbound Reunion", 2021, []int{35, 10751},
		"Estranged siblings reunite during Christmas week.", "Christmas")
	if code := h.bootstrap("owner", "owner-pass"); code != http.StatusOK {
		t.Fatalf("bootstrap → %d, want 200", code)
	}
	admin := h.login("owner", "owner-pass")

	var submitted struct {
		JobID string `json:"jobId"`
	}
	decodeBody(t, h.do(http.MethodPost, "/v1/proposals",
		`{"description":"A cozy Christmas movie channel"}`, admin), &submitted)
	if submitted.JobID == "" {
		t.Fatal("submit returned no jobId")
	}
	_, prop := h.awaitProposal(admin, submitted.JobID)
	if len(prop.Acquisitions) != 1 || prop.Acquisitions[0].TMDBID != 2401 {
		t.Fatalf("submitted holiday proposal did not include grounded outside-Library discovery: %+v", prop)
	}
	if prop.Policy.Seasonal.Mode != "exclusive" {
		t.Fatalf("seasonal mode = %q, want exclusive", prop.Policy.Seasonal.Mode)
	}
	stored, err := h.store.GetJob(context.Background(), submitted.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "done" {
		t.Fatalf("proposal job status = %q, want done", stored.Status)
	}
	// No approve call was made: the new acquisition remains submitted for review.
	proposals, err := h.store.ListProposalsByStatus(context.Background(), "submitted")
	if err != nil || len(proposals) != 1 || proposals[0].Status != "submitted" {
		t.Fatalf("holiday proposal bypassed or missed review: proposals=%+v err=%v", proposals, err)
	}
}

// decodeBody decodes a 2xx JSON response body into v (fails on non-2xx).
func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s → %d", resp.Request.URL.Path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// waitFor polls cond up to ~2s, failing with msg if it never holds.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}
