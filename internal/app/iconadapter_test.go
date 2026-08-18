package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// iconTestBase is the public URL the test image service builds rendition URLs from, so assertions
// can compare against the exact string a client would receive.
const iconTestBase = "https://loomarr.test"

// tmdbPoster is the source URL the adapter resolves a poster path to. `original`, not `w500`,
// since V52 phase 7 — see the imageBase comment in internal/tmdb.
func tmdbPoster(path string) string { return "https://image.tmdb.org/t/p/original" + path }

// iconPosterURL is the image-service URL a warm suggestion carries.
func iconPosterURL(hash string) string {
	return iconTestBase + "/v1/images/" + hash + "/w" + strconv.Itoa(iconLogoWidth) + ".jpg?r=loomarr-rendition-v2"
}

// newIconAdapter wires an iconAdapter over a real image service, pre-seeding `warm` source URLs as
// already-fetched rows and returning the hash each was given.
//
// ⚠ **The fetcher's host allowlist is EMPTY on purpose.** FetchNow is a real download path, and a
// unit test must never touch the network (AGENTS.md). An empty allowlist makes `checkURL` refuse
// before any dial, so the cold branch is exercised honestly — a fetch is attempted and fails —
// without opening a socket. Anything that should appear as a suggestion is seeded as already
// fetched instead, which is also the steady state on a real instance after the first minute.
func newIconAdapter(t *testing.T, st store.Store, client *tmdb.Client, warm ...string) (iconAdapter, map[string]string) {
	t.Helper()
	ims := newMemImageStore()
	svc := newTestImageService(t, t.TempDir(), iconTestBase, ims)

	const hexDigits = "abcdef0123456789"
	hashes := make(map[string]string, len(warm))
	for i, src := range warm {
		if i >= len(hexDigits) {
			t.Fatalf("newIconAdapter: %d warm URLs exceeds the distinct-hash supply", len(warm))
		}
		h := strings.Repeat(string(hexDigits[i]), 64)
		hashes[src] = h
		if err := ims.PutImage(context.Background(), images.Image{
			Hash: h, Origin: images.OriginRemote, SourceURL: src,
			Role: images.RoleIcon, Visibility: images.VisibilityPublic,
			OriginFetchedAt: time.Unix(1_700_000_000, 0),
		}); err != nil {
			t.Fatal(err)
		}
	}

	f := images.NewFetcher(svc, ims,
		func() bool { return true },
		func() int { return 2 },
		func() []string { return nil }, // refuses every host — see above
		nil)
	return iconAdapter{store: st, tmdb: client, images: svc, fetch: f}, hashes
}

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
	if _, err := st.SaveChannel(context.Background(), store.Channel{
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

	st := testkit.MigratedSQLiteStore(t)
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "Star Trek: TNG"},
		{Key: "series:tvdb:2000", Title: "Star Trek: DS9"},
		{Key: "movie:tmdb:603", Title: "The Matrix"},
	})

	a, hashes := newIconAdapter(t, st, tmdb.NewWithBase(mock.URL, "key"),
		tmdbPoster("/tng.jpg"), tmdbPoster("/ds9.jpg"), tmdbPoster("/matrix.jpg"))
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("suggestions = %d, want 3: %+v", len(got), got)
	}
	byTitle := map[string]api.IconSuggestion{}
	for _, s := range got {
		byTitle[s.Title] = s
	}
	// ⚠ Every URL is OURS. A TMDB URL surviving here is the §22 regression this phase exists to
	// remove: the icon picker is one of the three surfaces that loaded third-party images in the
	// operator's browser.
	for _, c := range []struct{ title, src string }{
		{"Star Trek: TNG", tmdbPoster("/tng.jpg")},
		{"Star Trek: DS9", tmdbPoster("/ds9.jpg")},
		{"The Matrix", tmdbPoster("/matrix.jpg")},
	} {
		sg := byTitle[c.title]
		if want := iconPosterURL(hashes[c.src]); sg.URL != want {
			t.Errorf("%s poster url = %q, want %q", c.title, sg.URL, want)
		}
		if sg.ImageHash != hashes[c.src] {
			t.Errorf("%s image hash = %q, want %q", c.title, sg.ImageHash, hashes[c.src])
		}
		if strings.Contains(sg.URL, "image.tmdb.org") {
			t.Errorf("%s still points at TMDB: %q", c.title, sg.URL)
		}
	}
}

// ⚠ A poster whose bytes have not landed is OMITTED, never offered with a placeholder hash.
//
// Adopt keys a pending row on a hash of the source URL and the fetch re-keys it onto the content
// hash, deleting that row. A suggestion's `url` becomes the channel's stored `logo`, so offering
// the placeholder would mint a logo that resolves for under a minute and 404s forever after.
func TestIconAdapter_OmitsPostersWhoseBytesHaveNotLanded(t *testing.T) {
	mock := tmdbPosterStub(t, map[string]string{
		"/tv/1000": "/tng.jpg",
		"/tv/1001": "/ds9.jpg",
	}, nil)
	defer mock.Close()

	st := testkit.MigratedSQLiteStore(t)
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "Warm"},
		{Key: "series:tmdb:1001", Title: "Cold"},
	})

	// Only TNG is seeded as fetched; DS9 stays cold and its fetch is refused by the empty allowlist.
	a, hashes := newIconAdapter(t, st, tmdb.NewWithBase(mock.URL, "key"), tmdbPoster("/tng.jpg"))
	got, err := a.IconSuggestions(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Warm" {
		t.Fatalf("suggestions = %+v, want exactly the warm one", got)
	}
	if want := iconPosterURL(hashes[tmdbPoster("/tng.jpg")]); got[0].URL != want {
		t.Errorf("url = %q, want %q", got[0].URL, want)
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

	st := testkit.MigratedSQLiteStore(t)
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "Good Title"},
		{Key: "series:tmdb:1001", Title: "Bad Title"},
	})

	a, _ := newIconAdapter(t, st, tmdb.NewWithBase(mock.URL, "key"), tmdbPoster("/tng.jpg"))
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

	st := testkit.MigratedSQLiteStore(t)
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "No Poster"},
	})

	a, _ := newIconAdapter(t, st, tmdb.NewWithBase(mock.URL, "key"))
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

	st := testkit.MigratedSQLiteStore(t)
	upsertLineupChannel(t, st, "ch-1", []schedule.LineupEntry{
		{Key: "series:tmdb:1000", Title: "TNG"},
		{Key: "series:tmdb:99000", Title: "TNG (dup)"},
	})

	a, _ := newIconAdapter(t, st, tmdb.NewWithBase(mock.URL, "key"), tmdbPoster("/tng.jpg"))
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
	st := testkit.MigratedSQLiteStore(t)
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
