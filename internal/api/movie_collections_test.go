package api_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestMovieCollections_ResolvesVisibleMoviesAsOneMemberReadableRequest(t *testing.T) {
	resolver := &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{
		Result: api.MovieCollectionResolution{
			Complete: true,
			Collections: []api.MovieCollection{{
				TMDBID: 1241,
				Name:   "Harry Potter Collection",
				Members: []api.SearchCandidate{
					{MediaType: "movie", TMDBID: 671, Name: "Harry Potter and the Philosopher's Stone", Year: 2001, InLibrary: true, LibraryItemID: "lib-671"},
					{MediaType: "movie", TMDBID: 672, Name: "Harry Potter and the Chamber of Secrets", Year: 2002},
				},
			}},
		},
	}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth:             testAuthorizer{},
		Log:              log,
		MovieCollections: resolver,
		LiveConfig:       func(key string) string { return map[string]string{"tmdb.api_key": "key"}[key] },
	})

	req := httptest.NewRequest(http.MethodGet,
		"/v1/movie-collections?key=movie:tmdb:671&key=movie:tmdb:673", nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	requests := resolver.Requests()
	if len(requests) != 1 || len(requests[0].Keys) != 2 || requests[0].Keys[0] != "movie:tmdb:671" || requests[0].Keys[1] != "movie:tmdb:673" {
		t.Fatalf("requests = %+v, want one bounded request preserving both Keys", requests)
	}
	var body struct {
		Collections []api.MovieCollection `json:"collections"`
		Complete    bool                  `json:"complete"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Complete || len(body.Collections) != 1 || body.Collections[0].Name != "Harry Potter Collection" {
		t.Fatalf("body = %+v, want one complete authoritative collection", body)
	}
	if len(body.Collections[0].Members) != 2 || body.Collections[0].Members[0].LibraryItemID != "lib-671" {
		t.Fatalf("members = %+v, want release-ordered members with Library presence", body.Collections[0].Members)
	}
}

func TestMovieCollections_RejectsMissingOrInvalidMovieKeysBeforeCallingService(t *testing.T) {
	resolver := &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth:             testAuthorizer{},
		Log:              log,
		MovieCollections: resolver,
		LiveConfig:       func(string) string { return "key" },
	})

	for _, target := range []string{
		"/v1/movie-collections",
		"/v1/movie-collections?key=series:tmdb:1399",
		"/v1/movie-collections?key=movie:tvdb:671",
		"/v1/movie-collections?key=not-a-key",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", "Bearer "+memberToken)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400: %s", target, resp.Code, resp.Body.String())
		}
	}
	if requests := resolver.Requests(); len(requests) != 0 {
		t.Fatalf("requests = %+v, want invalid public input rejected before the service", requests)
	}
}

func TestMovieCollections_DistinguishesUnavailableConfigurationAndUpstreamFailure(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	for _, tc := range []struct {
		name       string
		service    api.MovieCollectionService
		liveConfig func(string) string
		wantStatus int
	}{
		{name: "service not wired", liveConfig: func(string) string { return "key" }, wantStatus: http.StatusNotImplemented},
		{name: "tmdb not configured", service: &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{}, liveConfig: func(string) string { return "" }, wantStatus: http.StatusNotImplemented},
		{name: "tmdb failed", service: &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{Err: errors.New("tmdb down")}, liveConfig: func(string) string { return "key" }, wantStatus: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := api.Router(log, api.Options{Auth: testAuthorizer{}, Log: log, MovieCollections: tc.service, LiveConfig: tc.liveConfig})
			req := httptest.NewRequest(http.MethodGet, "/v1/movie-collections?key=movie:tmdb:671", nil)
			req.Header.Set("Authorization", "Bearer "+memberToken)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", resp.Code, tc.wantStatus, resp.Body.String())
			}
		})
	}
}

func TestMovieCollections_NormalizesEmptyCollectionsToJSONArray(t *testing.T) {
	resolver := &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{
		Result: api.MovieCollectionResolution{Complete: true},
	}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: testAuthorizer{}, Log: log, MovieCollections: resolver,
		LiveConfig: func(string) string { return "key" },
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/movie-collections?key=movie:tmdb:550", nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	collections, ok := body["collections"].([]any)
	if !ok || len(collections) != 0 {
		t.Fatalf("collections = %#v, want []", body["collections"])
	}
}

func TestMovieCollections_BoundsVisibleKeys(t *testing.T) {
	resolver := &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: testAuthorizer{}, Log: log, MovieCollections: resolver,
		LiveConfig: func(string) string { return "key" },
	})
	target := "/v1/movie-collections?"
	for i := 1; i <= 25; i++ {
		if i > 1 {
			target += "&"
		}
		target += "key=movie:tmdb:" + strconv.Itoa(i)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+memberToken)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", resp.Code, resp.Body.String())
	}
	if requests := resolver.Requests(); len(requests) != 0 {
		t.Fatalf("requests = %+v, want oversized input rejected before the service", requests)
	}
}

func TestMovieCollections_RequiresAuthentication(t *testing.T) {
	resolver := &testkit.MovieCollectionService[api.MovieCollectionRequest, api.MovieCollectionResolution]{}
	log := slog.New(slog.DiscardHandler)
	handler := api.Router(log, api.Options{
		Auth: testAuthorizer{}, Log: log, MovieCollections: resolver,
		LiveConfig: func(string) string { return "key" },
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/movie-collections?key=movie:tmdb:671", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", resp.Code, resp.Body.String())
	}
	if requests := resolver.Requests(); len(requests) != 0 {
		t.Fatalf("requests = %+v, want authorization before service work", requests)
	}
}
