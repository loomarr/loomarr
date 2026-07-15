package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeSettings is a scripted api.SettingsService for the route tests.
type fakeSettings struct {
	patched map[string]string
	patchBy string
	regen   string // last regenerated secret name
}

func (f *fakeSettings) List(context.Context) []api.SettingEntry {
	return []api.SettingEntry{
		{Key: "library.url", Group: "connections.media_server", Kind: "url", Value: "http://emby:8096", Provenance: "db", Doc: "x"},
		{Key: "library.token", Group: "connections.media_server", Kind: "secret", Secret: true, Set: true, Preview: "…a1b2", Provenance: "db", Doc: "x"},
		{Key: "job.workers", Group: "advanced", Kind: "int", Value: "2", Provenance: "env", Doc: "x"},
	}
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
	return out
}

func (f *fakeSettings) Features(context.Context) map[string]bool {
	return map[string]bool{"suggestions": false, "acquisition": true, "filler": false}
}

func (f *fakeSettings) RegenerateSecret(_ context.Context, name string) (string, bool, error) {
	f.regen = name
	if name == "session_secret" {
		return "", false, nil // never displayable
	}
	return "brand-new-token-value", true, nil
}

func (f *fakeSettings) Test(_ context.Context, check string) (bool, string) {
	if check == "media_server" {
		return true, ""
	}
	return false, "not configured"
}

func newSettingsServer(t *testing.T) (*httptest.Server, *fakeSettings) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/s.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fs := &fakeSettings{}
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     api.NewTokenAuthorizer(adminToken),
		Log:      slog.New(slog.DiscardHandler),
		Settings: fs,
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
		{http.MethodPost, "/v1/settings/secrets/api_token/regenerate", ""},
	} {
		resp := do(t, srv, tc.method, tc.path, "", tc.body) // empty token → not admin
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s without admin → %d, want 403", tc.method, tc.path, resp.StatusCode)
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

// Regenerating SESSION_SECRET withholds the value (config-design §4); a
// displayable secret returns it.
func TestSettings_SecretRegenerationDisplayPolicy(t *testing.T) {
	srv, _ := newSettingsServer(t)

	// session_secret → withheld.
	resp := do(t, srv, http.MethodPost, "/v1/settings/secrets/session_secret/regenerate", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("regen session → %d", resp.StatusCode)
	}
	var s struct {
		Value       string `json:"value"`
		Displayable bool   `json:"displayable"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&s)
	if s.Displayable || s.Value != "" {
		t.Errorf("session secret must be withheld: %+v", s)
	}

	// api_token → returned.
	resp = do(t, srv, http.MethodPost, "/v1/settings/secrets/api_token/regenerate", adminToken, "")
	var a struct {
		Value       string `json:"value"`
		Displayable bool   `json:"displayable"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&a)
	if !a.Displayable || a.Value == "" {
		t.Errorf("api_token should return the new value: %+v", a)
	}
}
