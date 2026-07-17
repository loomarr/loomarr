package integration_test

import (
	"net/http"
	"testing"
)

// TestWiring_FreshInstall pins the composition-root contract for a store-only
// "nothing configured yet" install — the FE's initial state. It asserts the two
// distinct nil-dep behaviors from ONE real app build: store-alone routes work
// (onboarding, settings, status, backup), feature/scheduler routes report
// unconfigured (501), and a route whose dep is absent is 404 (not registered).
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

	// Feature/scheduler routes report UNCONFIGURED (501) — no library/tunarr/llm.
	// (After the live-enable fix these still 501 when unconfigured; saving the config
	// makes them work live — see TestWiring_ConfigEnablesLive.)
	unconfigured := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/suggestions", `{"description":"x"}`},
		{http.MethodGet, "/v1/search?q=matrix", ""},
		{http.MethodPost, "/v1/channels/x/reconcile", ""},
		{http.MethodPost, "/v1/filler/sync", ""},
		{http.MethodPost, "/v1/setup/livetv-connect", ""},
	}
	for _, r := range unconfigured {
		if code := h.status(r.method, r.path, r.body, admin); code != http.StatusNotImplemented {
			t.Errorf("fresh install: %s %s → %d, want 501 (unconfigured)", r.method, r.path, code)
		}
	}

	// A dep that's ABSENT (no library ⇒ no user-sync route registered) is NOT a 501:
	// the POST is unhandled and the /v1/users/{id} pattern shadows the path, so the
	// router answers 405. A different contract than 501, which the FE must not treat
	// as "configured but broken."
	if code := h.status(http.MethodPost, "/v1/users/sync", "", admin); code != http.StatusMethodNotAllowed {
		t.Errorf("fresh install: POST /v1/users/sync → %d, want 405 (route absent)", code)
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
		{http.MethodPost, "/v1/suggestions", `{"description":"x"}`},
		{http.MethodGet, "/v1/search?q=matrix", ""},
		{http.MethodPost, "/v1/channels/x/reconcile", ""},
	}
	for _, r := range preCheck {
		if code := h.status(r.method, r.path, r.body, admin); code != http.StatusNotImplemented {
			t.Fatalf("pre-config %s %s → %d, want 501", r.method, r.path, code)
		}
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
	if code := h.status(http.MethodPost, "/v1/suggestions", `{"description":"x"}`, admin); code == http.StatusNotImplemented {
		t.Error("POST /v1/suggestions still 501 after saving config — not live-enabled (would need a restart)")
	}
	if code := h.status(http.MethodGet, "/v1/search?q=matrix", "", admin); code != http.StatusOK {
		t.Errorf("GET /v1/search → %d after saving library, want 200 (live-enabled)", code)
	}
	// The scheduler gate opened: reconcile of a missing channel is now 404, not 501.
	if code := h.status(http.MethodPost, "/v1/channels/x/reconcile", "", admin); code == http.StatusNotImplemented {
		t.Error("reconcile still 501 after saving tunarr.url — scheduler not live-enabled")
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
