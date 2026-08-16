package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// fakeSettings is a scripted api.SettingsService for the route tests.
type fakeSettings struct {
	patched  map[string]string
	patchBy  string
	regen    string // last regenerated secret name
	revealed string // last revealed secret name
	cleared  string // last key passed to Clear

	envOverride   string // last key passed to SetEnvOverride (§3.1)
	envOverrideOn bool   // whether that call claimed the key or handed it back

	afterPatch       func(map[string]string)
	afterClear       func(string)
	afterEnvOverride func(string, bool)
}

func TestSettings_UsesDurableBackendTransitionAfterEffectiveWrites(t *testing.T) {
	cfg := map[string]string{"playout.backend": schedule.PlayoutBackendInternal}
	settings := &fakeSettings{
		afterPatch: func(edits map[string]string) {
			if value, ok := edits["playout.backend"]; ok {
				cfg["playout.backend"] = value
			}
		},
		afterClear: func(string) { cfg["playout.backend"] = schedule.PlayoutBackendInternal },
		afterEnvOverride: func(_ string, enabled bool) {
			if !enabled {
				cfg["playout.backend"] = schedule.PlayoutBackendTunarr
			}
		},
	}
	transition := &testkit.BackendTransition{
		Err: context.DeadlineExceeded,
		Desired: func() string {
			return cfg["playout.backend"]
		},
	}
	live := &fakeConnector{}
	st := openTestStore(t, t.TempDir()+"/transition.db")
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(adminToken), Settings: settings,
		LiveTV: live, BackendTransition: transition,
		LiveConfig: func(key string) string { return cfg[key] },
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	requests := []struct {
		method, path, body string
		wantStatus         int
	}{
		{http.MethodPatch, "/v1/settings", `{"edits":{"playout.backend":"tunarr"}}`, http.StatusOK},
		{http.MethodDelete, "/v1/settings/playout.backend", "", http.StatusNoContent},
		{http.MethodPut, "/v1/settings/playout.backend/env-override", `{"enabled":false}`, http.StatusNoContent},
	}
	for _, request := range requests {
		resp := do(t, srv, request.method, request.path, adminToken, request.body)
		if resp.StatusCode != request.wantStatus {
			t.Fatalf("%s %s = %d, want %d", request.method, request.path, resp.StatusCode, request.wantStatus)
		}
	}
	// Rotating the device token repairs internal URLs because they embed it. Other
	// generated tokens have no Live TV consequence.
	for _, name := range []string{"playout_token", "api_token"} {
		resp := do(t, srv, http.MethodPost, "/v1/settings/secrets/"+name+"/regenerate", adminToken, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("regenerate %s = %d", name, resp.StatusCode)
		}
	}
	if got := transition.Targets(); len(got) != 4 || got[0] != schedule.PlayoutBackendTunarr ||
		got[1] != schedule.PlayoutBackendInternal || got[2] != schedule.PlayoutBackendTunarr ||
		got[3] != schedule.PlayoutBackendTunarr {
		t.Fatalf("transition targets = %v", got)
	}
	if live.calls != 0 || live.wiredChecks != 0 {
		t.Fatalf("legacy Live TV path ran with durable transition: calls=%d checks=%d", live.calls, live.wiredChecks)
	}
	if settings.patched["playout.backend"] != schedule.PlayoutBackendTunarr {
		t.Fatalf("post-save transition failure lost durable PATCH result: %v", settings.patched)
	}
}

func TestSettings_BackendTransitionLockFailureDoesNotRunMutation(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		mutated                  func(*fakeSettings) bool
	}{
		{
			name: "patch", method: http.MethodPatch, path: "/v1/settings",
			body:    `{"edits":{"playout.backend":"internal"}}`,
			mutated: func(s *fakeSettings) bool { return s.patched != nil },
		},
		{
			name: "clear", method: http.MethodDelete, path: "/v1/settings/playout.backend",
			mutated: func(s *fakeSettings) bool { return s.cleared != "" },
		},
		{
			name: "environment hand-back", method: http.MethodPut,
			path: "/v1/settings/playout.backend/env-override", body: `{"enabled":false}`,
			mutated: func(s *fakeSettings) bool { return s.envOverride != "" },
		},
		{
			name: "playout token rotation", method: http.MethodPost,
			path:    "/v1/settings/secrets/playout_token/regenerate",
			mutated: func(s *fakeSettings) bool { return s.regen != "" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := &fakeSettings{}
			transition := &testkit.BackendTransition{BeforeMutationErr: context.DeadlineExceeded}
			h := api.Router(slog.New(slog.DiscardHandler), api.Options{
				Auth: api.NewTokenAuthorizer(adminToken), Settings: settings, BackendTransition: transition,
			})
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			resp := do(t, srv, tc.method, tc.path, adminToken, tc.body)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("request with unavailable transition lock = %d, want 503", resp.StatusCode)
			}
			if tc.mutated(settings) {
				t.Fatal("mutation ran without transition lock")
			}
		})
	}
}

func TestSettings_MissingBackendTransitionDoesNotRunMutation(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		mutated                  func(*fakeSettings) bool
	}{
		{
			name: "patch", method: http.MethodPatch, path: "/v1/settings",
			body:    `{"edits":{"playout.backend":"internal"}}`,
			mutated: func(s *fakeSettings) bool { return s.patched != nil },
		},
		{
			name: "clear", method: http.MethodDelete, path: "/v1/settings/playout.backend",
			mutated: func(s *fakeSettings) bool { return s.cleared != "" },
		},
		{
			name: "environment hand-back", method: http.MethodPut,
			path: "/v1/settings/playout.backend/env-override", body: `{"enabled":false}`,
			mutated: func(s *fakeSettings) bool { return s.envOverride != "" },
		},
		{
			name: "playout token rotation", method: http.MethodPost,
			path:    "/v1/settings/secrets/playout_token/regenerate",
			mutated: func(s *fakeSettings) bool { return s.regen != "" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := &fakeSettings{}
			h := api.Router(slog.New(slog.DiscardHandler), api.Options{
				Auth: api.NewTokenAuthorizer(adminToken), Settings: settings,
			})
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			resp := do(t, srv, tc.method, tc.path, adminToken, tc.body)
			if resp.StatusCode != http.StatusNotImplemented {
				t.Fatalf("request without transition workflow = %d, want 501", resp.StatusCode)
			}
			if tc.mutated(settings) {
				t.Fatal("mutation ran without transition workflow")
			}
		})
	}
}

func (f *fakeSettings) List(context.Context) []api.SettingEntry {
	return []api.SettingEntry{
		{Key: "library.url", Group: "connections.media_server", Kind: "url", Apply: "live", Value: "http://emby:8096", Provenance: "db", Doc: "x"},
		{Key: "library.token", Group: "connections.media_server", Kind: "secret", Apply: "live", Secret: true, Set: true, Preview: "…a1b2", Provenance: "db", Doc: "x"},
		{Key: "job.workers", Group: "advanced", Kind: "int", Apply: "live", Value: "2", Provenance: "env", Doc: "x"},
	}
}

// Clear scripts the three outcomes the route maps to HTTP: unknown → invalid (404),
// env-pinned → pinned (409), anything else → saved (204).
func (f *fakeSettings) Clear(_ context.Context, key string) api.SettingResult {
	f.cleared = key
	switch key {
	case "nope.missing":
		return api.SettingResult{Key: key, Status: "invalid", Problem: "unknown setting"}
	case "job.workers":
		return api.SettingResult{Key: key, Status: "pinned", Problem: "set via environment"}
	}
	if f.afterClear != nil {
		f.afterClear(key)
	}
	return api.SettingResult{Key: key, Status: "saved"}
}

// SetEnvOverride scripts §3.1's three outcomes: an unregistered key (bootstrap keys
// included) → unknown (404), a key the environment does not pin → not_pinned (409),
// otherwise applied (204).
func (f *fakeSettings) SetEnvOverride(_ context.Context, key string, on bool, _ string) api.SettingResult {
	f.envOverride = key
	f.envOverrideOn = on
	switch key {
	case "nope.missing":
		return api.SettingResult{Key: key, Status: "unknown"}
	case "library.url":
		// Provenance "db" in the fixture above — nothing in the environment to take back.
		return api.SettingResult{Key: key, Status: "not_pinned"}
	}
	if f.afterEnvOverride != nil {
		f.afterEnvOverride(key, on)
	}
	return api.SettingResult{Key: key, Status: "applied"}
}

func (f *fakeSettings) Patch(_ context.Context, edits map[string]string, by string) []api.SettingResult {
	f.patched = edits
	f.patchBy = by
	var out []api.SettingResult
	for k := range edits {
		st := "saved"
		if k == "job.workers" {
			st = "pinned" // env-pinned
		}
		out = append(out, api.SettingResult{Key: k, Status: st})
	}
	if f.afterPatch != nil {
		f.afterPatch(edits)
	}
	return out
}

func (f *fakeSettings) Features(context.Context) map[string]bool {
	return map[string]bool{"suggestions": false, "acquisition": true, "filler": false}
}

func (f *fakeSettings) RegenerateSecret(_ context.Context, name string) (string, error) {
	f.regen = name
	return "brand-new-token-value", nil
}

func (f *fakeSettings) RevealSecret(_ context.Context, name string) (string, error) {
	f.revealed = name
	return "current-secret-value", nil
}

func (f *fakeSettings) Test(_ context.Context, check string) (bool, string) {
	if check == "media_server" {
		return true, ""
	}
	return false, "not configured"
}

func newSettingsServer(t *testing.T) (*httptest.Server, *fakeSettings) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/s.db")
	t.Cleanup(func() { _ = st.Close() })
	fs := &fakeSettings{}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:             st,
		Auth:              testAuthorizer{},
		Log:               slog.New(slog.DiscardHandler),
		Settings:          fs,
		BackendTransition: &testkit.BackendTransition{},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, fs
}

// Every settings route is admin-only (config-design §8, §19): a non-admin → 403.
func TestSettings_RequireAdmin(t *testing.T) {
	srv, _ := newSettingsServer(t)
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/v1/settings", ""},
		{http.MethodPatch, "/v1/settings", `{"edits":{"library.url":"http://x:1"}}`},
		{http.MethodPost, "/v1/setup/test", `{"check":"media_server"}`},
		{http.MethodDelete, "/v1/settings/library.token", ""},
		{http.MethodGet, "/v1/settings/secrets/api_token", ""},
		{http.MethodPost, "/v1/settings/secrets/api_token/regenerate", ""},
		// §3.1's unlock is the one control that overrides the deploy config, so the §19
		// negative matters more here than anywhere else on this surface.
		{http.MethodPut, "/v1/settings/seerr.url/env-override", `{"enabled":true}`},
	} {
		resp := do(t, srv, tc.method, tc.path, "", tc.body) // empty token → not admin
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without admin → %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// GET /v1/settings masks secrets (no value; set+preview only) and returns the
// computed feature set (config-design §7, §8).
func TestSettings_ListMasksSecretsAndFeatures(t *testing.T) {
	srv, _ := newSettingsServer(t)
	resp := do(t, srv, http.MethodGet, "/v1/settings", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list → %d", resp.StatusCode)
	}
	var body struct {
		Settings []api.SettingEntry `json:"settings"`
		Features map[string]bool    `json:"features"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	for _, e := range body.Settings {
		if e.Secret {
			if e.Value != "" {
				t.Errorf("secret %s leaked value %q", e.Key, e.Value)
			}
			if !e.Set || e.Preview == "" {
				t.Errorf("secret %s should carry set+preview", e.Key)
			}
		}
	}
	if body.Features["suggestions"] { // fake says false
		t.Error("features not surfaced")
	}
}

// PATCH returns per-key results and threads the admin id (config-design §3, §8).
func TestSettings_PatchResults(t *testing.T) {
	srv, fs := newSettingsServer(t)
	resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
		`{"edits":{"library.url":"http://emby:8096","job.workers":"9"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch → %d", resp.StatusCode)
	}
	var body struct {
		Results []api.SettingResult `json:"results"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	got := map[string]string{}
	for _, r := range body.Results {
		got[r.Key] = r.Status
	}
	if got["library.url"] != "saved" || got["job.workers"] != "pinned" {
		t.Errorf("patch results: %+v", got)
	}
	if fs.patched["library.url"] != "http://emby:8096" {
		t.Error("edits not passed through")
	}
}

func TestSettings_GeneratedTokenRegenerationReturnsValue(t *testing.T) {
	srv, _ := newSettingsServer(t)
	for _, name := range []string{"api_token", "playout_token"} {
		resp := do(t, srv, http.MethodPost, "/v1/settings/secrets/"+name+"/regenerate", adminToken, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("regenerate %s → %d", name, resp.StatusCode)
		}
		var body struct {
			Value       string `json:"value"`
			Displayable *bool  `json:"displayable"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Value == "" || body.Displayable != nil {
			t.Errorf("%s should return the new value: %+v", name, body)
		}
	}
}

// DELETE /v1/settings/{key} is the explicit clear (config-design §8) — the only way
// to unset a secret, since an empty-string PATCH on one is rejected (§9). Its three
// outcomes map to distinct statuses so a client can tell them apart.
func TestSettings_ClearOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		want      int
	}{
		{"clears a stored secret", "library.token", http.StatusNoContent},
		{"unknown key", "nope.missing", http.StatusNotFound},
		{"env-pinned key wins", "job.workers", http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, fs := newSettingsServer(t)
			resp := do(t, srv, http.MethodDelete, "/v1/settings/"+tc.key, adminToken, "")
			if resp.StatusCode != tc.want {
				t.Errorf("DELETE %s → %d, want %d", tc.key, resp.StatusCode, tc.want)
			}
			if fs.cleared != tc.key {
				t.Errorf("service saw key %q, want %q", fs.cleared, tc.key)
			}
		})
	}
}

// PUT /v1/settings/{key}/env-override is §3.1's unlock: the three service outcomes map to
// HTTP, and the claim direction reaches the service verbatim.
func TestSettings_EnvOverrideOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		enabled   bool
		want      int
	}{
		{"takes a pinned key back", "job.workers", true, http.StatusNoContent},
		{"hands a key back", "job.workers", false, http.StatusNoContent},
		// Bootstrap keys (DATABASE_URL et al) arrive here as unknown: they are read before
		// the database opens, so a flag stored IN that database could not affect them.
		{"unknown key", "nope.missing", true, http.StatusNotFound},
		{"nothing in the environment to take over", "library.url", true, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, fs := newSettingsServer(t)
			body := `{"enabled":false}`
			if tc.enabled {
				body = `{"enabled":true}`
			}
			resp := do(t, srv, http.MethodPut, "/v1/settings/"+tc.key+"/env-override", adminToken, body)
			if resp.StatusCode != tc.want {
				t.Errorf("PUT %s → %d, want %d", tc.key, resp.StatusCode, tc.want)
			}
			if fs.envOverride != tc.key {
				t.Errorf("service saw key %q, want %q", fs.envOverride, tc.key)
			}
			// The direction is load-bearing: a lock arriving as an unlock would silently do
			// the opposite of what the operator clicked.
			if fs.envOverrideOn != tc.enabled {
				t.Errorf("service saw enabled=%v, want %v", fs.envOverrideOn, tc.enabled)
			}
		})
	}
}

// GET /v1/settings/secrets/{name} is §4's eye toggle and never rotates.
func TestSettings_SecretReveal(t *testing.T) {
	srv, fs := newSettingsServer(t)

	for _, name := range []string{"api_token", "playout_token"} {
		resp := do(t, srv, http.MethodGet, "/v1/settings/secrets/"+name, adminToken, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("reveal %s → %d", name, resp.StatusCode)
		}
		var body struct {
			Value       string `json:"value"`
			Displayable *bool  `json:"displayable"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Value == "" || body.Displayable != nil {
			t.Errorf("%s should be revealable: %+v", name, body)
		}
	}

	// The point of the route: revealing must NOT rotate.
	if fs.regen != "" {
		t.Errorf("reveal rotated %q — reading a secret must never change it", fs.regen)
	}
}

func TestSettings_RetiredSessionSecretRoutesAreRejected(t *testing.T) {
	srv, fs := newSettingsServer(t)
	const oldName = "session_secret" // retired-ok: rejected compatibility probe for the removed setting
	for _, path := range []string{
		"/v1/settings/secrets/" + oldName,
		"/v1/settings/secrets/" + oldName + "/regenerate",
	} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/regenerate") {
			method = http.MethodPost
		}
		resp := do(t, srv, method, path, adminToken, "")
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s %s = %d, want 422", method, path, resp.StatusCode)
		}
	}
	if fs.regen != "" || fs.revealed != "" {
		t.Fatalf("retired secret reached settings adapter: regen=%q reveal=%q", fs.regen, fs.revealed)
	}
}

// fakeConnector records whether Connect fired and can be scripted to fail. It
// stands in for both wiring connectors (LiveTV + media source) — enough surface
// to prove the auto-wire fires (and stays non-fatal) after a connection save.
type fakeConnector struct {
	calls       int
	fail        bool
	beforeCall  func()
	wired       bool
	wiredErr    error
	wiredChecks int
}

func (c *fakeConnector) Connect(context.Context) (bool, bool, error) {
	if c.beforeCall != nil {
		c.beforeCall()
	}
	c.calls++
	if c.fail {
		return false, false, context.DeadlineExceeded
	}
	return true, true, nil // something changed (tuner + listing added)
}

func (c *fakeConnector) Wired(context.Context) (bool, error) {
	c.wiredChecks++
	return c.wired, c.wiredErr
}
func (c *fakeConnector) Reconnect(context.Context) (int, error) { return 0, nil }

func (c *fakeConnector) ConnectSource(context.Context) (string, int, error) {
	c.calls++
	if c.fail {
		return "", 0, context.DeadlineExceeded
	}
	return "src1", 2, nil
}

// mediaSourceAdapter satisfies api.TunarrConnector (Connect returns id+count).
type mediaSourceAdapter struct{ inner *fakeConnector }

func (a mediaSourceAdapter) Connect(ctx context.Context) (string, int, error) {
	return a.inner.ConnectSource(ctx)
}

func (a mediaSourceAdapter) LibrariesReady(context.Context) (bool, error) { return false, nil }

func newAutoWireServer(t *testing.T, live, source *fakeConnector, cfg map[string]string) *httptest.Server {
	srv, _ := newAutoWireServerWithChannels(t, live, source, cfg, &fakeSettings{}, nil)
	return srv
}

func newAutoWireServerWithChannels(t *testing.T, live, source *fakeConnector, cfg map[string]string,
	settings *fakeSettings, channelSvc api.ChannelService) (*httptest.Server, store.Store) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/s.db")
	t.Cleanup(func() { _ = st.Close() })
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:         st,
		Auth:          api.NewTokenAuthorizer(adminToken),
		Log:           slog.New(slog.DiscardHandler),
		Settings:      settings,
		Channels:      channelSvc,
		LiveTV:        live,
		TunarrConnect: mediaSourceAdapter{inner: source},
		BackendTransition: &testkit.BackendTransition{Desired: func() string {
			return cfg["playout.backend"]
		}},
		LiveConfig: func(k string) string { return cfg[k] },
		LibraryConfigured: func() bool {
			return cfg["library.flavor"] != "" && cfg["library.url"] != "" && cfg["library.token"] != ""
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

// Saving a connection enters the durable transition seam and independently wires the Tunarr
// media source. Live TV itself must never be wired by a duplicate HTTP-layer algorithm.
func TestSettings_AutoWiresAfterConnectionSave(t *testing.T) {
	configured := map[string]string{
		"tunarr.url": "http://tunarr:8000", "library.flavor": "emby",
		"library.url": "http://emby:8096", "library.token": "token",
	}

	t.Run("a connection save fires both wiring actions", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		srv := newAutoWireServer(t, live, source, configured)
		resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"tunarr.url":"http://tunarr:8000"}}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("patch → %d", resp.StatusCode)
		}
		if live.calls != 0 || source.calls != 1 {
			t.Errorf("wiring ownership drifted: legacy-live=%d source=%d", live.calls, source.calls)
		}
	})

	t.Run("a non-connection save wires nothing", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		srv := newAutoWireServer(t, live, source, configured)
		do(t, srv, http.MethodPatch, "/v1/settings", adminToken, `{"edits":{"job.workers":"9"}}`)
		if live.calls != 0 || source.calls != 0 {
			t.Errorf("touched no connection key but wired: live=%d source=%d", live.calls, source.calls)
		}
	})

	t.Run("a wiring failure never fails the save", func(t *testing.T) {
		live, source := &fakeConnector{fail: true}, &fakeConnector{fail: true}
		srv := newAutoWireServer(t, live, source, configured)
		resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"library.url":"http://emby:8096"}}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("a wiring failure leaked into the save: %d", resp.StatusCode)
		}
	})

	t.Run("live TV wires even before the media server is set", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		// Only Tunarr configured — livetv needs just tunarr.url; media source needs both.
		srv := newAutoWireServer(t, live, source, map[string]string{"tunarr.url": "http://tunarr:8000"})
		do(t, srv, http.MethodPatch, "/v1/settings", adminToken, `{"edits":{"tunarr.url":"http://tunarr:8000"}}`)
		if live.calls != 0 {
			t.Errorf("HTTP layer invoked legacy Live TV wiring: %d", live.calls)
		}
		if source.calls != 0 {
			t.Errorf("media source should wait for a complete library connection: %d", source.calls)
		}
	})

	t.Run("an incomplete media server triple wires nothing", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		cfg := map[string]string{
			"tunarr.url": "http://tunarr:8000", "library.flavor": "jellyfin",
			"library.url": "http://jellyfin:8096",
		}
		srv := newAutoWireServer(t, live, source, cfg)
		do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"library.url":"http://jellyfin:8096"}}`)
		if live.calls != 0 || source.calls != 0 {
			t.Errorf("incomplete library connection wired: legacy-live=%d source=%d", live.calls, source.calls)
		}
	})

	t.Run("internal backend wires without Tunarr", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		cfg := map[string]string{
			"playout.backend":   "internal",
			"server.public_url": "http://loomarr:8080",
		}
		srv := newAutoWireServer(t, live, source, cfg)
		resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"playout.backend":"internal"}}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("patch → %d", resp.StatusCode)
		}
		if live.calls != 0 {
			t.Errorf("HTTP layer invoked legacy Live TV wiring: %d", live.calls)
		}
		if source.calls != 0 {
			t.Errorf("playout.backend must not trigger Tunarr media-source wiring: %d", source.calls)
		}
	})

	t.Run("public URL change rewires internal Live TV", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		cfg := map[string]string{
			"playout.backend":   "internal",
			"server.public_url": "http://loomarr-new:8080",
		}
		srv := newAutoWireServer(t, live, source, cfg)
		do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"server.public_url":"http://loomarr-new:8080"}}`)
		if live.calls != 0 {
			t.Errorf("HTTP layer invoked legacy Live TV wiring: %d", live.calls)
		}
		if source.calls != 0 {
			t.Errorf("server.public_url must not trigger Tunarr media-source wiring: %d", source.calls)
		}
	})

	t.Run("internal Live TV waits for a reachable public URL", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		srv := newAutoWireServer(t, live, source, map[string]string{"playout.backend": "internal"})
		do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"playout.backend":"internal"}}`)
		if live.calls != 0 || source.calls != 0 {
			t.Errorf("unwireable internal backend invoked connectors: live=%d source=%d", live.calls, source.calls)
		}
	})

	t.Run("Tunarr Live TV still requires its URL", func(t *testing.T) {
		live, source := &fakeConnector{}, &fakeConnector{}
		srv := newAutoWireServer(t, live, source, map[string]string{"playout.backend": "tunarr"})
		do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"playout.backend":"tunarr"}}`)
		if live.calls != 0 || source.calls != 0 {
			t.Errorf("unconfigured Tunarr backend invoked connectors: live=%d source=%d", live.calls, source.calls)
		}
	})
}

func TestSettings_BackendTransitionDoesNotUseLegacyHTTPReconcileOrWiring(t *testing.T) {
	cfg := map[string]string{
		"playout.backend":   schedule.PlayoutBackendInternal,
		"server.public_url": "http://loomarr:8080",
		"tunarr.url":        "http://tunarr:8000",
	}
	settings := &fakeSettings{afterPatch: func(edits map[string]string) {
		for key, value := range edits {
			cfg[key] = value
		}
	}}
	channelSvc := &fakeChannelSvc{}
	live, source := &fakeConnector{}, &fakeConnector{}
	reconciledBeforeWire := -1
	live.beforeCall = func() { reconciledBeforeWire = len(channelSvc.reconciledIDs) }
	srv, st := newAutoWireServerWithChannels(t, live, source, cfg, settings, channelSvc)
	seedChannel(t, st, "inherits", "Follows default", 1, "")
	seedChannel(t, st, "pinned", "Pinned internal", 2, schedule.PlayoutBackendInternal)

	resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
		`{"edits":{"playout.backend":"tunarr"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch → %d", resp.StatusCode)
	}
	if reconciledBeforeWire != -1 {
		t.Fatalf("legacy Live TV wiring ran: before=%d", reconciledBeforeWire)
	}
	if len(channelSvc.reconciledIDs) != 0 {
		t.Fatalf("HTTP layer reconciled channels outside transition module: %v", channelSvc.reconciledIDs)
	}
	if live.calls != 0 {
		t.Fatalf("HTTP layer wired Live TV outside transition module: %d", live.calls)
	}
}

func TestSettings_BackendTransitionFailureRemainsPostSaveAndRetryable(t *testing.T) {
	cfg := map[string]string{
		"playout.backend":   schedule.PlayoutBackendInternal,
		"server.public_url": "http://loomarr:8080",
		"tunarr.url":        "http://tunarr:8000",
	}
	settings := &fakeSettings{afterPatch: func(edits map[string]string) {
		for key, value := range edits {
			cfg[key] = value
		}
	}}
	channelSvc := &fakeChannelSvc{err: context.DeadlineExceeded}
	live, source := &fakeConnector{}, &fakeConnector{}
	srv, st := newAutoWireServerWithChannels(t, live, source, cfg, settings, channelSvc)
	seedChannel(t, st, "inherits", "Follows default", 1, "")

	resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
		`{"edits":{"playout.backend":"tunarr"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the setting save remains successful: %d", resp.StatusCode)
	}
	if len(channelSvc.reconciledIDs) != 0 || live.calls != 0 {
		t.Fatalf("HTTP layer performed legacy transition: reconciled=%v live=%d", channelSvc.reconciledIDs, live.calls)
	}

	// Retrying the same save remains successful and still crosses only the deep transition seam.
	channelSvc.err = nil
	channelSvc.reconciledIDs = nil
	resp = do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
		`{"edits":{"playout.backend":"tunarr"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retry patch → %d", resp.StatusCode)
	}
	if len(channelSvc.reconciledIDs) != 0 || live.calls != 0 {
		t.Fatalf("retry performed legacy transition: reconciled=%v live=%d", channelSvc.reconciledIDs, live.calls)
	}
}

func TestSettings_LiveTVRegistrationChecksAreOwnedByTransitionModule(t *testing.T) {
	configured := map[string]string{
		"playout.backend":   schedule.PlayoutBackendInternal,
		"server.public_url": "http://loomarr:8080",
	}

	t.Run("matching current URLs need neither convergence nor connect", func(t *testing.T) {
		channelSvc := &fakeChannelSvc{err: context.DeadlineExceeded}
		live, source := &fakeConnector{wired: true}, &fakeConnector{}
		srv, st := newAutoWireServerWithChannels(t, live, source, configured, &fakeSettings{}, channelSvc)
		seedChannel(t, st, "inherits", "Follows default", 1, "")

		resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"playout.backend":"internal"}}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("patch → %d", resp.StatusCode)
		}
		if live.wiredChecks != 0 || live.calls != 0 || channelSvc.reconciles != 0 {
			t.Fatalf("HTTP layer performed registration effects: checks=%d connects=%d reconciles=%d",
				live.wiredChecks, live.calls, channelSvc.reconciles)
		}
	})

	t.Run("lookup failure keeps existing registration untouched", func(t *testing.T) {
		channelSvc := &fakeChannelSvc{}
		live, source := &fakeConnector{wiredErr: context.DeadlineExceeded}, &fakeConnector{}
		srv, st := newAutoWireServerWithChannels(t, live, source, configured, &fakeSettings{}, channelSvc)
		seedChannel(t, st, "inherits", "Follows default", 1, "")

		resp := do(t, srv, http.MethodPatch, "/v1/settings", adminToken,
			`{"edits":{"playout.backend":"internal"}}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("the setting save remains successful: %d", resp.StatusCode)
		}
		if live.wiredChecks != 0 || live.calls != 0 || channelSvc.reconciles != 0 {
			t.Fatalf("HTTP layer performed registration effects: checks=%d connects=%d reconciles=%d",
				live.wiredChecks, live.calls, channelSvc.reconciles)
		}
	})
}

func TestSettings_ClearAndEnvOverrideHotApplyBackendChanges(t *testing.T) {
	t.Run("clear falls back to internal", func(t *testing.T) {
		cfg := map[string]string{
			"playout.backend":   "tunarr",
			"server.public_url": "http://loomarr:8080",
			"tunarr.url":        "http://tunarr:8000",
		}
		settings := &fakeSettings{afterClear: func(key string) {
			if key == "playout.backend" {
				cfg[key] = schedule.PlayoutBackendInternal
			}
		}}
		channelSvc := &fakeChannelSvc{}
		live, source := &fakeConnector{}, &fakeConnector{}
		seenBeforeWire := -1
		live.beforeCall = func() { seenBeforeWire = len(channelSvc.reconciledIDs) }
		srv, st := newAutoWireServerWithChannels(t, live, source, cfg, settings, channelSvc)
		seedChannel(t, st, "inherits", "Follows default", 1, "")

		resp := do(t, srv, http.MethodDelete, "/v1/settings/playout.backend", adminToken, "")
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("clear → %d", resp.StatusCode)
		}
		if seenBeforeWire != -1 || len(channelSvc.reconciledIDs) != 0 || live.calls != 0 {
			t.Fatalf("clear used legacy transition: before=%d reconciled=%v live=%d",
				seenBeforeWire, channelSvc.reconciledIDs, live.calls)
		}
	})

	t.Run("handing back to environment selects Tunarr", func(t *testing.T) {
		cfg := map[string]string{
			"playout.backend":   schedule.PlayoutBackendInternal,
			"server.public_url": "http://loomarr:8080",
			"tunarr.url":        "http://tunarr:8000",
		}
		settings := &fakeSettings{afterEnvOverride: func(key string, enabled bool) {
			if key == "playout.backend" && !enabled {
				cfg[key] = "tunarr"
			}
		}}
		channelSvc := &fakeChannelSvc{}
		live, source := &fakeConnector{}, &fakeConnector{}
		seenBeforeWire := -1
		live.beforeCall = func() { seenBeforeWire = len(channelSvc.reconciledIDs) }
		srv, st := newAutoWireServerWithChannels(t, live, source, cfg, settings, channelSvc)
		seedChannel(t, st, "inherits", "Follows default", 1, "")

		resp := do(t, srv, http.MethodPut, "/v1/settings/playout.backend/env-override", adminToken,
			`{"enabled":false}`)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("env hand-back → %d", resp.StatusCode)
		}
		if seenBeforeWire != -1 || len(channelSvc.reconciledIDs) != 0 || live.calls != 0 {
			t.Fatalf("env hand-back used legacy transition: before=%d reconciled=%v live=%d",
				seenBeforeWire, channelSvc.reconciledIDs, live.calls)
		}
	})
}

