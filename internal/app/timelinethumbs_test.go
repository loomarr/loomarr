package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/images"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// tmdbTimelineStub serves the TMDB endpoints the timeline resolver uses: a movie's backdrop
// (/movie/{id}), a series episode's still (/tv/{id}/season/{s}/episode/{e}), and the TVDB→TMDB
// bridge (/find/{tvdbId}?external_source=tvdb_id → a tv_result carrying the TMDB series id).
// `finds` maps a /find path to the TMDB series id it resolves to. Paths not in the maps 404.
func tmdbTimelineStub(t *testing.T, backdrops, stills map[string]string, finds map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/find/"):
			if id, ok := finds["/find/"+strings.TrimPrefix(r.URL.Path, "/find/")]; ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"tv_results": []map[string]int{{"id": id}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tv_results": []any{}})
		case strings.Contains(r.URL.Path, "/season/"): // /tv/{id}/season/{s}/episode/{e}
			if p, ok := stills[r.URL.Path]; ok {
				_ = json.NewEncoder(w).Encode(map[string]string{"still_path": p})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]string{"backdrop_path": backdrops[r.URL.Path]})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// ⚠ `original`, not `w500`, since V52 phase 7: the resolver ADOPTS this URL rather than handing it
// to a browser, and §22 fetches the original once and builds the width ladder locally.
const imgBase = "https://image.tmdb.org/t/p/original"

func newTimelineTMDB(t *testing.T) *tmdb.Client {
	t.Helper()
	mock := tmdbTimelineStub(t,
		map[string]string{"/movie/603": "/matrix-wide.jpg"},
		// Episode stills, keyed by the TMDB series id (456 direct; 789 reached via the tvdb bridge).
		map[string]string{
			"/tv/456/season/1/episode/6": "/lisa.jpg",
			"/tv/789/season/2/episode/1": "/bart.jpg",
		},
		// The TVDB→TMDB bridge: tvdb 71663 → tmdb series 789.
		map[string]int{"/find/71663": 789},
	)
	t.Cleanup(mock.Close)
	return tmdb.NewWithBase(mock.URL, "key")
}

// The key→TMDB-URL resolution the strip depends on: a series (tmdb OR tvdb key) resolves to its
// EPISODE's still, a movie to its landscape backdrop, and a TVDB series bridges TVDB→TMDB via
// /find first.
//
// It is asserted through ADOPTION rather than on a returned TMDB URL, because the resolver no
// longer returns one. Seeding a fetched row per expected source URL is what makes each branch
// observable: the resolver only emits a URL for an image whose bytes exist, so the row it finds
// proves which TMDB URL it resolved the key to.
func TestTimelineThumbResolver(t *testing.T) {
	st := newMemImageStore()
	svc := newTestImageService(t.TempDir(), "https://machine-client-only.invalid", st)
	ctx := context.Background()

	// A fetched row per source URL the resolver should arrive at. The hashes are arbitrary but
	// must be valid sha256 hex, since that is what a rendition URL is allowed to contain.
	seeded := map[string]string{
		imgBase + "/lisa.jpg":        strings.Repeat("a", 64),
		imgBase + "/matrix-wide.jpg": strings.Repeat("b", 64),
		imgBase + "/bart.jpg":        strings.Repeat("c", 64),
	}
	for src, hash := range seeded {
		if err := st.PutImage(ctx, images.Image{
			Hash: hash, Origin: images.OriginRemote, SourceURL: src,
			Role: images.RoleBackdrop, Visibility: images.VisibilityMember,
			OriginFetchedAt: time.Unix(1_700_000_000, 0),
		}); err != nil {
			t.Fatal(err)
		}
	}

	r := timelineThumbResolver{tmdb: newTimelineTMDB(t), images: svc}

	cases := []struct {
		name            string
		key             string
		season, episode int
		wantHash        string
	}{
		{"tmdb series → episode still", "series:tmdb:456", 1, 6, seeded[imgBase+"/lisa.jpg"]},
		{"movie → backdrop", "movie:tmdb:603", 0, 0, seeded[imgBase+"/matrix-wide.jpg"]},
		// A series episode TMDB doesn't have (404) → "", not an error.
		{"series with no still", "series:tmdb:456", 9, 9, ""},
		// The COMMON case (§3 series prefer a TVDB key): bridge TVDB→TMDB, then the episode still.
		{"tvdb series → bridged episode still", "series:tvdb:71663", 2, 1, seeded[imgBase+"/bart.jpg"]},
		// A TVDB series TMDB has no match for → "" (no error).
		{"tvdb series with no TMDB match", "series:tvdb:2000", 1, 1, ""},
		{"empty key", "", 0, 0, ""},
		{"malformed key", "not-a-key", 0, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotHash := r.ThumbFor(ctx, c.key, c.season, c.episode)
			if gotHash != c.wantHash {
				t.Fatalf("ThumbFor(%q, %d, %d) hash = %q, want %q", c.key, c.season, c.episode, gotHash, c.wantHash)
			}
			wantURL := ""
			if c.wantHash != "" {
				wantURL = "/v1/images/" + c.wantHash + "/w300.jpg"
			}
			if gotURL != wantURL {
				t.Errorf("ThumbFor(%q, %d, %d) url = %q, want %q", c.key, c.season, c.episode, gotURL, wantURL)
			}
		})
	}
}

type timelineFetchStub struct {
	work []images.Image
	warm map[string]images.Image
}

func (f *timelineFetchStub) FetchNow(_ context.Context, work []images.Image, _ time.Duration) map[string]images.Image {
	f.work = append(f.work, work...)
	return f.warm
}

// ⚠ An image the strip has never seen is adopted and warmed inside the interactive request. The
// returned URL is still ours, never TMDB's, but the first hover must not wait for a scheduler tick.
//
// This is the seam §22 exists to close: the timeline used to hand the operator's browser a
// third-party URL, which is a beacon telling TMDB who is watching what and when. The honest
// degradation on a failed bounded fetch is no image — never a gray permanent placeholder.
func TestTimelineThumbWarmsAColdImageBeforeReturning(t *testing.T) {
	st := newMemImageStore()
	contentHash := strings.Repeat("d", 64)
	src := imgBase + "/lisa.jpg"
	fetch := &timelineFetchStub{warm: map[string]images.Image{
		src: {
			Hash: contentHash, SourceURL: src, Role: images.RoleBackdrop,
			OriginFetchedAt: time.Unix(1_700_000_000, 0),
		},
	}}
	r := timelineThumbResolver{
		tmdb: newTimelineTMDB(t), images: newTestImageService(t.TempDir(), "https://machine-client-only.invalid", st),
		fetch: fetch,
	}

	url, hash := r.ThumbFor(context.Background(), "series:tmdb:456", 1, 6)
	if want := "/v1/images/" + contentHash + "/w300.jpg"; url != want || hash != contentHash {
		t.Errorf("ThumbFor on a cold image = (%q, %q), want (%q, %q)", url, hash, want, contentHash)
	}
	if len(fetch.work) != 1 || fetch.work[0].SourceURL != src || !fetch.work[0].OriginFetchedAt.IsZero() {
		t.Fatalf("FetchNow work = %+v, want the one cold adopted still", fetch.work)
	}
}

// Without an image service the resolver returns nothing rather than falling back to TMDB's CDN.
func TestTimelineThumbWithoutImageServiceEmitsNothing(t *testing.T) {
	r := timelineThumbResolver{tmdb: newTimelineTMDB(t)}
	if url, hash := r.ThumbFor(context.Background(), "series:tmdb:456", 1, 6); url != "" || hash != "" {
		t.Errorf("ThumbFor with no image service = (%q, %q), want empty — never a third-party URL", url, hash)
	}
}
