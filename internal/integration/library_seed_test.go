package integration_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// libraryMovie is one movie in Loomarr's own Library, as a scan would leave it: the titles the
// catalog search, the availability scan and the durations read (#2151 step 16).
type libraryMovie struct {
	ID      string
	Name    string
	Year    int
	TMDBID  int
	Genres  []string
	Rating  string
	Runtime time.Duration
}

// kidsCartoons are four in-library cartoons with real ratings and runtimes. Every name holds
// "cartoon", the term the scripted LLM searches, so the model sees them all: enforcement, not
// search, must keep the TV-MA one off a kids channel.
var kidsCartoons = []libraryMovie{
	{ID: "lib-1", Name: "Sunny Cartoon Hour", Year: 1992, TMDBID: 90005001, Genres: []string{"Animation"}, Rating: "TV-Y7", Runtime: time.Hour},
	{ID: "lib-2", Name: "Robo Rangers Cartoon", Year: 1993, TMDBID: 90005002, Genres: []string{"Animation"}, Rating: "TV-Y7", Runtime: time.Hour},
	{ID: "lib-3", Name: "Critter Cartoon Club", Year: 1994, TMDBID: 90005003, Genres: []string{"Animation"}, Rating: "TV-Y", Runtime: time.Hour},
	{ID: "lib-4", Name: "Midnight Mayhem Cartoons", Year: 1994, TMDBID: 90005004, Genres: []string{"Animation"}, Rating: "TV-MA", Runtime: time.Hour},
}

// seedLibraryMovies writes movies, each with one present, read file, into the Library tables.
func seedLibraryMovies(t *testing.T, st store.Store, movies ...libraryMovie) {
	t.Helper()
	at := time.Unix(1_800_000_000, 0).UTC()
	items := make([]store.LibraryItem, 0, len(movies))
	files := make([]store.LibraryFile, 0, len(movies))
	for _, m := range movies {
		items = append(items, store.LibraryItem{
			ID: m.ID, Kind: "movie", Name: m.Name, Year: m.Year, Rating: m.Rating, Genres: m.Genres,
			RuntimeMs: m.Runtime.Milliseconds(),
			IDs:       []store.LibraryID{{Namespace: "tmdb", Value: strconv.Itoa(m.TMDBID)}},
		})
		files = append(files, store.LibraryFile{
			Path: "/movies/" + m.ID + ".mkv", Root: "/movies", Size: 1, ModifiedNs: 1, Kind: "movie",
			Header: store.LibraryHeaderRead, SeenAt: at, ItemID: m.ID,
		})
	}
	ctx := context.Background()
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
}
