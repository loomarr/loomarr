package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/testkit"
)

func newConfiguredSearchHandler(
	t *testing.T,
	cfg map[string]string,
) (http.Handler, *testkit.SearchService[api.SearchCandidate]) {
	t.Helper()
	search := &testkit.SearchService[api.SearchCandidate]{Results: []api.SearchCandidate{{
		MediaType: "movie", TMDBID: 603, Name: "The Matrix",
	}}}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth:   testAuthorizer{},
		Log:    log,
		Search: search,
		LiveConfig: func(key string) string {
			return cfg[key]
		},
		LibraryConfigured: func() bool {
			return cfg["library.flavor"] != "" && cfg["library.url"] != "" && cfg["library.token"] != ""
		},
	})
	return handler, search
}

func searchRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func TestSearchScopesFollowLiveConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		cfg       map[string]string
		wantCode  int
		wantScope string
	}{
		{name: "library configured", path: "/v1/search?q=matrix&scope=library", cfg: libraryConfig(), wantCode: http.StatusOK, wantScope: "library"},
		{name: "library missing", path: "/v1/search?q=matrix&scope=library", cfg: map[string]string{"tmdb.api_key": "key"}, wantCode: http.StatusNotImplemented},
		{name: "tmdb configured", path: "/v1/search?q=matrix&scope=tmdb", cfg: map[string]string{"tmdb.api_key": "key"}, wantCode: http.StatusOK, wantScope: "tmdb"},
		{name: "tmdb missing", path: "/v1/search?q=matrix&scope=tmdb", cfg: libraryConfig(), wantCode: http.StatusNotImplemented},
		{name: "all with both", path: "/v1/search?q=matrix&scope=all", cfg: libraryConfig("tmdb.api_key", "key"), wantCode: http.StatusOK, wantScope: "all"},
		{name: "all narrows to library", path: "/v1/search?q=matrix&scope=all", cfg: libraryConfig(), wantCode: http.StatusOK, wantScope: "library"},
		{name: "library triple incomplete", path: "/v1/search?q=matrix&scope=library", cfg: map[string]string{"library.flavor": "jellyfin", "library.url": "http://library"}, wantCode: http.StatusNotImplemented},
		{name: "default narrows to tmdb", path: "/v1/search?q=matrix", cfg: map[string]string{"tmdb.api_key": "key"}, wantCode: http.StatusOK, wantScope: "tmdb"},
		{name: "neither configured", path: "/v1/search?q=matrix", cfg: map[string]string{}, wantCode: http.StatusNotImplemented},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, search := newConfiguredSearchHandler(t, tt.cfg)
			resp := searchRequest(handler, tt.path)
			if resp.Code != tt.wantCode {
				t.Fatalf("search status = %d, want %d", resp.Code, tt.wantCode)
			}
			requests := search.Requests()
			if tt.wantCode != http.StatusOK {
				if len(requests) != 0 {
					t.Fatalf("unconfigured search reached adapter: %+v", requests)
				}
				return
			}
			if len(requests) != 1 || requests[0].Scope != tt.wantScope {
				t.Fatalf("search requests = %+v, want one %q scope", requests, tt.wantScope)
			}
		})
	}
}

func libraryConfig(extra ...string) map[string]string {
	cfg := map[string]string{
		"library.flavor": "jellyfin", "library.url": "http://library", "library.token": "token",
	}
	for i := 0; i+1 < len(extra); i += 2 {
		cfg[extra[i]] = extra[i+1]
	}
	return cfg
}

func TestSearchConfigurationHotApplies(t *testing.T) {
	cfg := map[string]string{}
	handler, search := newConfiguredSearchHandler(t, cfg)

	resp := searchRequest(handler, "/v1/search?q=matrix")
	if resp.Code != http.StatusNotImplemented {
		t.Fatalf("unconfigured search = %d, want 501", resp.Code)
	}

	cfg["tmdb.api_key"] = "key"
	resp = searchRequest(handler, "/v1/search?q=matrix")
	if resp.Code != http.StatusOK {
		t.Fatalf("search after setting TMDB key = %d, want 200", resp.Code)
	}

	delete(cfg, "tmdb.api_key")
	for key, value := range libraryConfig() {
		cfg[key] = value
	}
	resp = searchRequest(handler, "/v1/search?q=matrix")
	if resp.Code != http.StatusOK {
		t.Fatalf("search after switching to library = %d, want 200", resp.Code)
	}

	requests := search.Requests()
	if len(requests) != 2 || requests[0].Scope != "tmdb" || requests[1].Scope != "library" {
		t.Fatalf("hot-applied scopes = %+v, want tmdb then library", requests)
	}
}
