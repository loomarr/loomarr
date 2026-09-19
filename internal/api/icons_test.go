package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type iconsHarness struct {
	*apiHarness
	Handler http.Handler
	Icons   *testkit.IconService[api.IconSuggestion]
}

func newIconsHarness(t *testing.T) *iconsHarness {
	return startIconsHarness(t, nil, true)
}

func newConfiguredIconsHarness(t *testing.T, config map[string]string) *iconsHarness {
	return startIconsHarness(t, config, true)
}

func newIconsWithoutServiceHarness(t *testing.T) *iconsHarness {
	return startIconsHarness(t, nil, false)
}

func startIconsHarness(t *testing.T, config map[string]string, withService bool) *iconsHarness {
	t.Helper()
	var icons *testkit.IconService[api.IconSuggestion]
	var iconService api.IconService
	if withService {
		icons = &testkit.IconService[api.IconSuggestion]{}
		iconService = icons
	}
	var handler http.Handler
	base := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		var liveConfig func(string) string
		if config != nil {
			liveConfig = func(key string) string { return config[key] }
		}
		handler = api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log,
			Icons: iconService, LiveConfig: liveConfig,
		})
		return handler
	})
	if _, err := base.Store.SaveChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Star Trek", Number: 42, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	return &iconsHarness{apiHarness: base, Handler: handler, Icons: icons}
}

func iconsRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

// The endpoint renders the candidate posters the service resolved from the channel's
// own lineup — e.g. a Star Trek channel offering its five series' posters (§icon P2).
func TestChannelIconSuggestions_RendersCandidates(t *testing.T) {
	harness := newIconsHarness(t)
	srv, fi := harness.Server, harness.Icons
	fi.Results = []api.IconSuggestion{
		{Title: "Star Trek: The Next Generation", URL: "https://image.tmdb.org/t/p/w500/tng.jpg"},
		{Title: "Star Trek: Deep Space Nine", URL: "https://image.tmdb.org/t/p/w500/ds9.jpg"},
	}

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("icon-suggestions → %d, want 200", resp.StatusCode)
	}
	var body struct {
		Suggestions []api.IconSuggestion `json:"suggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Suggestions) != 2 {
		t.Fatalf("suggestions = %d, want 2", len(body.Suggestions))
	}
	if body.Suggestions[0].Title != "Star Trek: The Next Generation" || body.Suggestions[0].URL != "https://image.tmdb.org/t/p/w500/tng.jpg" {
		t.Errorf("suggestion[0] = %+v", body.Suggestions[0])
	}
	if asked := fi.ChannelIDs(); len(asked) != 1 || asked[0] != "ch-1" {
		t.Errorf("asked = %v, want exactly [ch-1]", asked)
	}
}

// An empty candidate set must render as [] rather than null, so the FE renders an
// empty state instead of guarding a case that never means failure.
func TestChannelIconSuggestions_EmptyIsEmptyArray(t *testing.T) {
	harness := newIconsHarness(t)
	srv, fi := harness.Server, harness.Icons
	fi.Results = nil

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("icon-suggestions → %d, want 200", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["suggestions"]); got != "[]" {
		t.Errorf("suggestions = %s, want [] (never null)", got)
	}
}

// Read-only: any authenticated user may fetch icon suggestions, matching get-channel.
func TestChannelIconSuggestions_VisibleToAnyAuthenticatedUser(t *testing.T) {
	srv := newIconsHarness(t).Server
	if resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", memberToken, ""); resp.StatusCode != http.StatusOK {
		t.Errorf("member request → %d, want 200 (read-only)", resp.StatusCode)
	}
}

func TestChannelIconSuggestions_UnknownChannelIs404(t *testing.T) {
	srv := newIconsHarness(t).Server
	if resp := do(t, srv, http.MethodGet, "/v1/channels/nope/icon-suggestions", adminToken, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown channel → %d, want 404", resp.StatusCode)
	}
}

// With no icon service wired (TMDB unconfigured), the route reports unavailable
// rather than 500 — the same nil-service contract every optional TMDB-gated feature
// follows (search, suggest, …).
func TestChannelIconSuggestions_501WhenNoService(t *testing.T) {
	srv := newIconsWithoutServiceHarness(t).Server

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon-suggestions", adminToken, "")
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("icon-suggestions with no service → %d, want 501", resp.StatusCode)
	}
}

func TestChannelIconSuggestionsFollowLiveTMDBKey(t *testing.T) {
	cfg := map[string]string{}
	harness := newConfiguredIconsHarness(t, cfg)
	handler, icons := harness.Handler, harness.Icons

	resp := iconsRequest(handler, "/v1/channels/ch-1/icon-suggestions")
	if resp.Code != http.StatusNotImplemented {
		t.Fatalf("icon suggestions without TMDB key = %d, want 501", resp.Code)
	}
	if asked := icons.ChannelIDs(); len(asked) != 0 {
		t.Fatalf("unconfigured icon request reached adapter: %v", asked)
	}

	cfg["tmdb.api_key"] = "key"
	resp = iconsRequest(handler, "/v1/channels/ch-1/icon-suggestions")
	if resp.Code != http.StatusOK {
		t.Fatalf("icon suggestions after setting TMDB key = %d, want 200", resp.Code)
	}
	if asked := icons.ChannelIDs(); len(asked) != 1 || asked[0] != "ch-1" {
		t.Fatalf("adapter calls after hot-apply = %v, want [ch-1]", asked)
	}
}
