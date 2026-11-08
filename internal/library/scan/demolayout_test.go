package scan

import (
	"context"
	"testing"
)

// The demo library's folders (demolibrary.Layout: FilmFile, EpisodeFile) are named so this scan
// grounds them with no arr and no search: a film and a show by the id in their names, the show's
// episodes by their numbers. The demo plays only what this finds (#2151 step 16), so the two
// shapes are pinned here as well as in the demo's own layout test.
func TestTheDemoLibrarysLayoutIsGroundedByItsNames(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.file("movies/Return- of the Kite (1986) {tmdb-90000703}/Return- of the Kite (1986) {tmdb-90000703}.mp4", 10)
	h.file("shows/Captain Pickle (1987) {tmdb-90000701} {tvdb-90000701}/Season 01/Captain Pickle - S01E01.mp4", 10)
	h.file("shows/Captain Pickle (1987) {tmdb-90000701} {tvdb-90000701}/Season 01/Captain Pickle - S01E02.mp4", 10)
	h.run()

	film, ok, err := h.st.LibraryTitleByProviderID(ctx, "movie", "tmdb", "90000703")
	if err != nil || !ok {
		t.Fatalf("film by its tmdb id: ok=%v err=%v", ok, err)
	}
	if film.Kind != "movie" {
		t.Fatalf("film = %+v", film)
	}
	show, ok, err := h.st.LibraryTitleByProviderID(ctx, "series", "tvdb", "90000701")
	if err != nil || !ok {
		t.Fatalf("show by its tvdb id: ok=%v err=%v", ok, err)
	}
	eps, found, err := h.st.LibraryEpisodes(ctx, show.ID)
	if err != nil || !found {
		t.Fatalf("episodes: found=%v err=%v", found, err)
	}
	if len(eps) != 2 || eps[0].Season != 1 || eps[0].Episode != 1 || eps[1].Episode != 2 {
		t.Fatalf("episodes = %+v, want S01E01 and S01E02", eps)
	}
}
