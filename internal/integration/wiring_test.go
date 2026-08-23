package integration_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
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

// TestWiring_LibraryConfigHotApplies proves one app generation keeps every optional
// media-library adapter wired while its complete connection snapshot changes live. The
// same handler starts empty, consumes a saved Jellyfin triple, observes a secret rotation,
// and returns to a dormant/no-outbound state after clear.
func TestWiring_LibraryConfigHotApplies(t *testing.T) {
	h := newHarness(t, withoutConnections(), withFillerStorage())
	admin := h.asAdmin()
	if err := h.store.UpsertFillerSource(t.Context(),
		store.NewFillerSource("library-movies", "library", "Movies", "Commercials", time.Now())); err != nil {
		t.Fatal(err)
	}

	assertLibrarySearchStatus(t, h, admin, http.StatusNotImplemented)
	assertUserImportStatus(t, h, admin, http.StatusNotImplemented)
	assertUserSyncStatus(t, h, admin, http.StatusNotImplemented)
	assertFillerSyncStatus(t, h, admin, http.StatusOK)
	if got := len(h.ms.Requests()); got != 0 {
		t.Fatalf("empty connection made %d media-server requests, want none", got)
	}

	if code := h.patchSettings(admin, map[string]string{
		"library.flavor": "jellyfin",
		"library.url":    h.ms.URL,
		"library.token":  "jellyfin-token-one",
	}); code != http.StatusOK {
		t.Fatalf("configure Jellyfin connection → %d, want 200", code)
	}
	assertLibrarySearchStatus(t, h, admin, http.StatusOK)
	assertUserImportStatus(t, h, admin, http.StatusOK)
	assertUserSyncStatus(t, h, admin, http.StatusOK)
	assertFillerSyncStatus(t, h, admin, http.StatusOK)
	requests := h.ms.Requests()
	assertJellyfinSearchAuth(t, requests, "jellyfin-token-one")
	assertJellyfinUserAuth(t, requests, "jellyfin-token-one")
	assertFillerLibraryScanned(t, requests, "jellyfin-token-one")

	if code := h.patchSettings(admin, map[string]string{"library.token": "jellyfin-token-two"}); code != http.StatusOK {
		t.Fatalf("rotate Jellyfin token → %d, want 200", code)
	}
	assertLibrarySearchStatus(t, h, admin, http.StatusOK)
	assertUserImportStatus(t, h, admin, http.StatusOK)
	requests = h.ms.Requests()
	assertJellyfinSearchAuth(t, requests, "jellyfin-token-two")
	assertJellyfinUserAuth(t, requests, "jellyfin-token-two")

	beforeClear := len(requests)
	if code := h.status(http.MethodDelete, "/v1/settings/library.token", "", admin); code != http.StatusNoContent {
		t.Fatalf("clear library.token → %d, want 204", code)
	}
	assertLibrarySearchStatus(t, h, admin, http.StatusNotImplemented)
	assertUserImportStatus(t, h, admin, http.StatusNotImplemented)
	assertUserSyncStatus(t, h, admin, http.StatusNotImplemented)
	assertFillerSyncStatus(t, h, admin, http.StatusOK)
	if got := len(h.ms.Requests()); got != beforeClear {
		t.Fatalf("cleared connection made an outbound request: before=%d after=%d", beforeClear, got)
	}
}

func assertUserImportStatus(t *testing.T, h *harness, admin *http.Cookie, want int) {
	t.Helper()
	if got := h.status(http.MethodGet, "/v1/users/candidates", "", admin); got != want {
		t.Fatalf("user import candidates = %d, want %d", got, want)
	}
}

func assertUserSyncStatus(t *testing.T, h *harness, admin *http.Cookie, want int) {
	t.Helper()
	if got := h.status(http.MethodPost, "/v1/users/sync", "", admin); got != want {
		t.Fatalf("user sync = %d, want %d", got, want)
	}
}

func assertFillerSyncStatus(t *testing.T, h *harness, admin *http.Cookie, want int) {
	t.Helper()
	if got := h.status(http.MethodPost, "/v1/filler/sync", "", admin); got != want {
		t.Fatalf("filler sync = %d, want %d", got, want)
	}
}

func assertLibrarySearchStatus(t *testing.T, h *harness, admin *http.Cookie, want int) {
	t.Helper()
	if got := h.status(http.MethodGet, "/v1/search?q=matrix&scope=library", "", admin); got != want {
		t.Fatalf("library search = %d, want %d", got, want)
	}
}

func assertFillerLibraryScanned(t *testing.T, requests []testkit.MediaServerRequest, token string) {
	t.Helper()
	foundFolder, foundItems := false, false
	for _, request := range requests {
		isFolder := request.Path == "/Library/VirtualFolders"
		isItems := request.Path == "/Items" && strings.Contains(request.RawQuery, "ParentId=")
		if isFolder {
			foundFolder = true
		}
		if isItems {
			foundItems = true
		}
		if isFolder || isItems {
			assertJellyfinAuthorization(t, "filler library request", request.Authorization, token)
		}
	}
	if !foundFolder || !foundItems {
		t.Fatalf("filler library scanner did not issue both lookup requests: %+v", requests)
	}
}

func assertJellyfinUserAuth(t *testing.T, requests []testkit.MediaServerRequest, token string) {
	t.Helper()
	for i := len(requests) - 1; i >= 0; i-- {
		request := requests[i]
		if request.Path != "/Users" {
			continue
		}
		assertJellyfinAuthorization(t, "Jellyfin user-list", request.Authorization, token)
		if request.EmbyToken != "" {
			t.Fatalf("Jellyfin user-list sent X-Emby-Token %q", request.EmbyToken)
		}
		return
	}
	t.Fatalf("no media-server user-list request found in %+v", requests)
}

func assertJellyfinSearchAuth(t *testing.T, requests []testkit.MediaServerRequest, token string) {
	t.Helper()
	for i := len(requests) - 1; i >= 0; i-- {
		request := requests[i]
		if request.Path != "/Items" || !strings.Contains(request.RawQuery, "SearchTerm=matrix") {
			continue
		}
		assertJellyfinAuthorization(t, "Jellyfin search", request.Authorization, token)
		if request.EmbyToken != "" {
			t.Fatalf("Jellyfin search sent X-Emby-Token %q", request.EmbyToken)
		}
		return
	}
	t.Fatalf("no media-server search request found in %+v", requests)
}

func assertJellyfinAuthorization(t *testing.T, surface, authorization, token string) {
	t.Helper()
	if !strings.HasPrefix(authorization, "MediaBrowser ") ||
		!strings.Contains(authorization, `Token="`+token+`"`) {
		t.Fatalf("%s Authorization = %q, want token %q", surface, authorization, token)
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

	// External-feature routes report UNCONFIGURED — no library/tunarr/llm. Proposal
	// submission uses the machine-readable 409 feature_not_configured contract; routes
	// with no live implementation retain the general 501 response.
	unconfigured := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/v1/proposals", `{"description":"x"}`, http.StatusConflict},
		{http.MethodGet, "/v1/search?q=matrix", "", http.StatusNotImplemented},
		{http.MethodPost, "/v1/setup/tunarr-connect", "", http.StatusNotImplemented},
	}
	for _, r := range unconfigured {
		if code := h.status(r.method, r.path, r.body, admin); code != r.want {
			t.Errorf("fresh install: %s %s → %d, want %d (unconfigured)", r.method, r.path, code, r.want)
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

	// Optional routes remain registered for the life of the process. With no complete
	// library triple, user sync reports unconfigured and makes no outbound request.
	if code := h.status(http.MethodPost, "/v1/users/sync", "", admin); code != http.StatusNotImplemented {
		t.Errorf("fresh install: POST /v1/users/sync → %d, want 501 (library unconfigured)", code)
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

	// Fresh install: the feature routes report unconfigured.
	preCheck := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/v1/proposals", `{"description":"x"}`, http.StatusConflict},
		{http.MethodGet, "/v1/search?q=matrix", "", http.StatusNotImplemented},
	}
	for _, r := range preCheck {
		if code := h.status(r.method, r.path, r.body, admin); code != r.want {
			t.Fatalf("pre-config %s %s → %d, want %d", r.method, r.path, code, r.want)
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
