package integration_test

import (
	"net/http"
	"testing"
)

// TestWiring_TMDBConfigHotApplies proves the composition root owns one real dynamic
// TMDB adapter rather than swapping in a boot-frozen test client. The sequence drives
// the public settings and connection-test APIs exactly as an operator does: an empty
// key sends no outbound request, configure and rotate affect the very next operation,
// and clear returns to the no-request state without rebuilding the handler.
func TestWiring_TMDBConfigHotApplies(t *testing.T) {
	h := newHarness(t, withoutConnections())
	admin := h.asAdmin()

	probe := func() (bool, string) {
		t.Helper()
		var out struct {
			OK   bool   `json:"ok"`
			Hint string `json:"hint"`
		}
		resp := h.do(http.MethodPost, "/v1/setup/test", `{"check":"tmdb"}`, admin)
		decodeBody(t, resp, &out)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST /v1/setup/test tmdb → %d, want 200", resp.StatusCode)
		}
		return out.OK, out.Hint
	}
	assertLastCredential := func(want string) {
		t.Helper()
		requests := h.tmdb.Requests()
		if len(requests) == 0 {
			t.Fatal("TMDB double received no request")
		}
		if got := requests[len(requests)-1].Authorization; got != "Bearer "+want {
			t.Fatalf("TMDB Authorization = %q, want bearer for newly saved credential", got)
		}
	}

	before := h.tmdb.RequestCount()
	if ok, hint := probe(); ok || hint != "set your TMDB API key" {
		t.Fatalf("empty TMDB probe = (%v, %q), want unconfigured", ok, hint)
	}
	if got := h.tmdb.RequestCount(); got != before {
		t.Fatalf("empty TMDB credential sent %d outbound requests, want none", got-before)
	}

	if code := h.patchSettings(admin, map[string]string{"tmdb.api_key": "first-key"}); code != http.StatusOK {
		t.Fatalf("configure TMDB key → %d, want 200", code)
	}
	if ok, hint := probe(); !ok {
		t.Fatalf("configured TMDB probe failed: %q", hint)
	}
	assertLastCredential("first-key")
	before = h.tmdb.RequestCount()
	if code := h.status(http.MethodGet, "/v1/search?q=matrix&scope=tmdb", "", admin); code != http.StatusOK {
		t.Fatalf("TMDB-only search after configure → %d, want 200 without a library connection", code)
	}
	if got := h.tmdb.RequestCount(); got != before+1 {
		t.Fatalf("TMDB-only search sent %d outbound requests, want one through the shared client", got-before)
	}
	assertLastCredential("first-key")

	if code := h.patchSettings(admin, map[string]string{"tmdb.api_key": "rotated-key"}); code != http.StatusOK {
		t.Fatalf("rotate TMDB key → %d, want 200", code)
	}
	if ok, hint := probe(); !ok {
		t.Fatalf("rotated TMDB probe failed: %q", hint)
	}
	assertLastCredential("rotated-key")

	before = h.tmdb.RequestCount()
	if code := h.status(http.MethodDelete, "/v1/settings/tmdb.api_key", "", admin); code != http.StatusNoContent {
		t.Fatalf("clear TMDB key → %d, want 204", code)
	}
	if ok, hint := probe(); ok || hint != "set your TMDB API key" {
		t.Fatalf("cleared TMDB probe = (%v, %q), want unconfigured", ok, hint)
	}
	if got := h.tmdb.RequestCount(); got != before {
		t.Fatalf("cleared TMDB credential sent %d outbound requests, want none", got-before)
	}
	if code := h.status(http.MethodGet, "/v1/search?q=matrix&scope=tmdb", "", admin); code != http.StatusNotImplemented {
		t.Fatalf("TMDB-only search after clear → %d, want 501 unconfigured", code)
	}
	if got := h.tmdb.RequestCount(); got != before {
		t.Fatalf("TMDB-only search after clear sent %d outbound requests, want none", got-before)
	}
}

// TestWiring_FreshInstall pins the composition-root contract for a store-only
// "nothing configured yet" install — the FE's initial state. It asserts the two
// distinct dependency behaviors from ONE real app build: store-alone routes work
// (onboarding, settings, status, backup), external-feature routes report unconfigured
// (501), and missing resources or routes are 404.
// This is the seam no other test covers: the whole api.Options wiring for an empty
// store, exactly as a freshly-installed backend answers the frontend.
func TestWiring_FreshInstall(t *testing.T) {
	h := newHarness(t, withoutConnections())
	admin := h.asAdmin() // bootstrap + local login work on the store alone

	// Store-alone routes WORK (200) — onboarding + settings never need a connection.
	works := []struct{ method, path string }{
		{http.MethodGet, "/v1/auth/me"},
		{http.MethodGet, "/v1/settings"},
		{http.MethodGet, "/v1/setup/status"},
		{http.MethodGet, "/v1/backup"},
	}
	for _, r := range works {
		if code := h.status(r.method, r.path, "", admin); code != http.StatusOK {
			t.Errorf("fresh install: %s %s → %d, want 200", r.method, r.path, code)
		}
	}

	// External-feature routes report UNCONFIGURED (501) — no library/tunarr/llm.
	// (After the live-enable fix these still 501 when unconfigured; saving the config
	// makes them work live — see TestWiring_ConfigEnablesLive.)
	unconfigured := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/proposals", `{"description":"x"}`},
		{http.MethodGet, "/v1/search?q=matrix", ""},
		{http.MethodPost, "/v1/setup/tunarr-connect", ""},
	}
	for _, r := range unconfigured {
		if code := h.status(r.method, r.path, r.body, admin); code != http.StatusNotImplemented {
			t.Errorf("fresh install: %s %s → %d, want 501 (unconfigured)", r.method, r.path, code)
		}
	}

	// Channel scheduling is local control-plane behavior on the default internal backend. It is
	// available without Tunarr; this id simply does not exist.
	if code := h.status(http.MethodPost, "/v1/channels/x/reconcile", "", admin); code != http.StatusNotFound {
		t.Errorf("fresh install: POST /v1/channels/x/reconcile → %d, want 404 (missing channel)", code)
	}

	// ⚠ Filler is NOT in that list, and the distinction is the point. `filler.dir` defaults
	// to /data/filler, so on a fresh install filler is CONFIGURED — sync therefore really
	// runs and fails upstream (no Tunarr yet), which is a 502, not the 501 that means "you
	// have not set this up". Asserting "not 501" rather than exactly 502 keeps this about
	// the gate: the claim is that filler no longer reports itself unconfigured out of the
	// box, which is what made the whole Filler page a single empty state.
	if code := h.status(http.MethodPost, "/v1/filler/sync", "", admin); code == http.StatusNotImplemented {
		t.Error("fresh install: POST /v1/filler/sync → 501; filler.dir has a default, so it is configured")
	}

	// A dep that's ABSENT (no library ⇒ no user-sync route registered) is NOT a 501:
	// the route simply doesn't exist. With the embedded SPA mounted as the "/"
	// catch-all (§12), an unhandled /v1 path is guarded to a uniform 404 (never
	// index.html) rather than the ServeMux method-shadow 405 it returned before the
	// FE landed. Either way the contract the FE relies on holds — route absent ≠ 501
	// ("configured but broken"), so the FE must not treat this as a feature that's on.
	if code := h.status(http.MethodPost, "/v1/users/sync", "", admin); code != http.StatusNotFound {
		t.Errorf("fresh install: POST /v1/users/sync → %d, want 404 (route absent, SPA-guarded)", code)
	}

	// The embedded SPA (§12) is served at / and client routes fall back to it —
	// PUBLIC (no cookie): the shell loads, then calls /v1/auth/me to decide login.
	// (In the test binary the embed is the .gitkeep placeholder or a real build;
	// either way the mount answers 200.)
	for _, path := range []string{"/", "/channels", "/settings"} {
		if code := h.status(http.MethodGet, path, "", nil); code != http.StatusOK {
			t.Errorf("SPA: GET %s → %d, want 200 (served shell, unauthenticated)", path, code)
		}
	}
}

// TestWiring_ConfigEnablesLive proves the FIX for the design-vs-code gap the audit
// found: saving a connection through the settings API enables its feature routes
// IMMEDIATELY, with no restart (config-design §3 hot-apply, §8.1). Before the fix a
// route stayed 501 until the process restarted even though GET /v1/settings already
// reported the feature "on" — exactly an FE surprise. This is the test that would
// have failed against the old boot-frozen wiring.
func TestWiring_ConfigEnablesLive(t *testing.T) {
	h := newHarness(t, withoutConnections())
	admin := h.asAdmin()

	// Fresh install: the feature routes report unconfigured (501).
	preCheck := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/proposals", `{"description":"x"}`},
		{http.MethodGet, "/v1/search?q=matrix", ""},
	}
	for _, r := range preCheck {
		if code := h.status(r.method, r.path, r.body, admin); code != http.StatusNotImplemented {
			t.Fatalf("pre-config %s %s → %d, want 501", r.method, r.path, code)
		}
	}
	// Reconcile is already live for internal playout; a missing channel is a resource miss, not
	// an unconfigured Tunarr feature.
	if code := h.status(http.MethodPost, "/v1/channels/x/reconcile", "", admin); code != http.StatusNotFound {
		t.Fatalf("pre-config reconcile missing channel → %d, want 404", code)
	}

	// SAVE the connections through the settings API — exactly what the wizard does.
	if code := h.patchSettings(admin, map[string]string{
		"library.flavor": "emby",
		"library.url":    h.ms.URL,
		"library.token":  h.ms.AdminToken,
		"tmdb.api_key":   "test-key",
		"llm.url":        h.ollama.URL,
		"llm.model":      "qwen3:8b",
		"tunarr.url":     h.tunarrStub.URL,
	}); code != http.StatusOK {
		t.Fatalf("PATCH /v1/settings → %d, want 200", code)
	}

	// The feature flag flips live in GET /v1/settings...
	if !h.features(admin)["suggestions"] {
		t.Error("features.suggestions did not flip on after saving LLM/TMDB config")
	}

	// ...AND the routes are enabled WITH NO RESTART — the whole point of the fix.
	if code := h.status(http.MethodPost, "/v1/proposals", `{"description":"x"}`, admin); code == http.StatusNotImplemented {
		t.Error("POST /v1/proposals still 501 after saving config — not live-enabled (would need a restart)")
	}
	if code := h.status(http.MethodGet, "/v1/search?q=matrix", "", admin); code != http.StatusOK {
		t.Errorf("GET /v1/search → %d after saving library, want 200 (live-enabled)", code)
	}
	// Saving external connections does not change the already-live channel scheduler contract.
	if code := h.status(http.MethodPost, "/v1/channels/x/reconcile", "", admin); code != http.StatusNotFound {
		t.Errorf("post-config reconcile missing channel → %d, want 404", code)
	}

	// The model-picker probe ALSO goes live — it reads the saved llm.url with no
	// restart. Regression for a live-smoke find: the picker's Prober was frozen at
	// boot, so after configuring Ollama through the wizard it still reported
	// unreachable until a restart.
	var sys struct {
		Reachable bool `json:"reachable"`
	}
	h.getJSON("/v1/system/llm", admin, &sys)
	if !sys.Reachable {
		t.Error("GET /v1/system/llm unreachable after saving llm.url — picker Prober not live (frozen at boot)")
	}
}
