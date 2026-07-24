package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// tmdbPosterStub is a minimal fake TMDB server covering just what iconAdapter needs:
// /movie/{id} and /tv/{id} poster_path, plus /find/{tvdb_id} for the TVDB bridge. Kept
// local rather than extending internal/testkit's TMDB mock, since no other subsystem
// needs poster fixtures yet (testkit.NewTMDB has no poster support to extend).
func tmdbPosterStub(t *testing.T, posters map[string]string, fail map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail[r.URL.Path] {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/movie/"), strings.HasPrefix(r.URL.Path, "/tv/"):
			p := posters[r.URL.Path]
			_ = json.NewEncoder(w).Encode(map[string]string{"poster_path": p})
		case strings.HasPrefix(r.URL.Path, "/find/"):
			p := posters[r.URL.Path]
			if p == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"tv_results": []any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tv_results": []map[string]string{{"poster_path": p}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func upsertLineupChannel(t *testing.T, st store.Store, id string, entries []schedule.LineupEntry) {
	t.Helper()
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: id, Name: "Test", Number: 1, Status: "live"},
		Lineup:  entries,
	}); err != nil {
		t.Fatal(err)
	}
}

// A themed channel (e.g. Star Trek) offers each series' own poster — the point of
// §icon P2: candidates come from the channel's OWN lineup, not a generic library.
func TestIconAdapter_ResolvesTMDBAndTVDBEntries(t *testing.T) {
	mock := tmdbPosterStub(t, map[string]string{
		"/tv/1000":   "/tng.jpg",
		"/movie/603": "/matrix.jpg",
		"/find/2000": "/ds9.jpg",
	}, nil)
	defer mock.Close()

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons.db", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "Star Trek: TNG"},
		{Key: "series:tvdb:2000", Title: "Star Trek: DS9"},
		{Key: "movie:tmdb:603", Title: "The Matrix"},
	})

	a := iconAdapter{store: st, tmdb: tmdb.NewWithBase(mock.URL, "key")}
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("suggestions = %d, want 3: %+v", len(got), got)
	}
	byTitle := map[string]string{}
	for _, s := range got {
		byTitle[s.Title] = s.URL
	}
	if byTitle["Star Trek: TNG"] != "https://image.tmdb.org/t/p/w500/tng.jpg" {
		t.Errorf("TNG poster = %q", byTitle["Star Trek: TNG"])
	}
	if byTitle["Star Trek: DS9"] != "https://image.tmdb.org/t/p/w500/ds9.jpg" {
		t.Errorf("DS9 (TVDB-bridged) poster = %q", byTitle["Star Trek: DS9"])
	}
	if byTitle["The Matrix"] != "https://image.tmdb.org/t/p/w500/matrix.jpg" {
		t.Errorf("Matrix poster = %q", byTitle["The Matrix"])
	}
}

// A per-title TMDB failure must be skipped, not fail the whole request — one bad
// title on a five-series channel shouldn't deny the other four suggestions.
func TestIconAdapter_SkipsFailingTitleBestEffort(t *testing.T) {
	mock := tmdbPosterStub(t,
		map[string]string{"/tv/1000": "/tng.jpg"},
		map[string]bool{"/tv/1001": true}, // this one 500s
	)
	defer mock.Close()

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons.db", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "Good Title"},
		{Key: "series:tmdb:1001", Title: "Bad Title"},
	})

	a := iconAdapter{store: st, tmdb: tmdb.NewWithBase(mock.URL, "key")}
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatalf("a bad title must not fail the whole request: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Good Title" {
		t.Errorf("suggestions = %+v, want exactly [Good Title]", got)
	}
}

// A title with no poster on TMDB is a legitimate empty result, not an error — it's
// simply omitted from the candidates.
func TestIconAdapter_SkipsEntriesWithNoPoster(t *testing.T) {
	mock := tmdbPosterStub(t, map[string]string{"/tv/1000": ""}, nil)
	defer mock.Close()

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons.db", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "No Poster"},
	})

	a := iconAdapter{store: st, tmdb: tmdb.NewWithBase(mock.URL, "key")}
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("suggestions = %+v, want none (no poster)", got)
	}
}

// Two lineup entries resolving to the same poster URL (a channel referencing the same
// title via more than one entry) must be de-duplicated.
func TestIconAdapter_DedupesByURL(t *testing.T) {
	mock := tmdbPosterStub(t, map[string]string{
		"/tv/1000":  "/tng.jpg",
		"/tv/99000": "/tng.jpg", // same poster under a different id, for the test
	}, nil)
	defer mock.Close()

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons.db", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "TNG"},
		{Key: "series:tmdb:99000", Title: "TNG (dup)"},
	})

	a := iconAdapter{store: st, tmdb: tmdb.NewWithBase(mock.URL, "key")}
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("suggestions = %+v, want 1 (deduped by URL)", got)
	}
}

// A malformed/legacy key is skipped, not fatal.
func TestIconAdapter_SkipsMalformedKeys(t *testing.T) {
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/icons.db", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: provision.Key("garbage"), Title: "Malformed"},
	})

	a := iconAdapter{store: st, tmdb: tmdb.NewWithBase("http://unused", "key")}
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("suggestions = %+v, want none", got)
	}
}

// Type-check: iconAdapter satisfies api.IconService.
var _ api.IconService = iconAdapter{}
