package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/tmdb"
)

// tmdbTimelineStub serves the TMDB endpoints the timeline resolver uses: a movie's poster
// (/movie/{id}), a series episode's still (/tv/{id}/season/{s}/episode/{e}), and the TVDB→TMDB
// bridge (/find/{tvdbId}?external_source=tvdb_id → a tv_result carrying the TMDB series id).
// `finds` maps a /find path to the TMDB series id it resolves to. Paths not in the maps 404.
func tmdbTimelineStub(t *testing.T, posters, stills map[string]string, finds map[string]int) *httptest.Server {
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
			_ = json.NewEncoder(w).Encode(map[string]string{"poster_path": posters[r.URL.Path]})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

const imgBase = "https://image.tmdb.org/t/p/w500"

// A series (tmdb OR tvdb key) resolves to its EPISODE's still; a movie to its poster. A TVDB series
// bridges TVDB→TMDB via /find first. Empty/malformed keys, and a TVDB series TMDB can't match, → "".
func TestTimelineThumbResolver(t *testing.T) {
	mock := tmdbTimelineStub(t,
		map[string]string{"/movie/603": "/matrix.jpg"},
		// Episode stills, keyed by the TMDB series id (456 direct; 789 reached via the tvdb bridge).
		map[string]string{
			"/tv/456/season/1/episode/6": "/lisa.jpg",
			"/tv/789/season/2/episode/1": "/bart.jpg",
		},
		// The TVDB→TMDB bridge: tvdb 71663 → tmdb series 789.
		map[string]int{"/find/71663": 789},
	)
	defer mock.Close()

	r := timelineThumbResolver{tmdb: tmdb.NewWithBase(mock.URL, "key")}

	cases := []struct {
		name            string
		key             string
		season, episode int
		want            string
	}{
		{"tmdb series → episode still", "series:tmdb:456", 1, 6, imgBase + "/lisa.jpg"},
		{"movie → poster", "movie:tmdb:603", 0, 0, imgBase + "/matrix.jpg"},
		// A series episode TMDB doesn't have (404) → "", not an error.
		{"series with no still", "series:tmdb:456", 9, 9, ""},
		// The COMMON case (§3 series prefer a TVDB key): bridge TVDB→TMDB, then the episode still.
		{"tvdb series → bridged episode still", "series:tvdb:71663", 2, 1, imgBase + "/bart.jpg"},
		// A TVDB series TMDB has no match for → "" (no error).
		{"tvdb series with no TMDB match", "series:tvdb:2000", 1, 1, ""},
		{"empty key", "", 0, 0, ""},
		{"malformed key", "not-a-key", 0, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.ThumbFor(context.Background(), c.key, c.season, c.episode); got != c.want {
				t.Errorf("ThumbFor(%q, %d, %d) = %q, want %q", c.key, c.season, c.episode, got, c.want)
			}
		})
	}
}
