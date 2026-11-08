package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

type titleSearchFake []store.LibraryTitle

func (f titleSearchFake) SearchLibraryTitles(_ context.Context, _ string, limit int) ([]store.LibraryTitle, error) {
	if len(f) > limit {
		return f[:limit], nil
	}
	return f, nil
}

type itemFactsFake map[string]store.LibraryItemFacts

func (f itemFactsFake) LibraryItemFacts(_ context.Context, ids []string) (map[string]store.LibraryItemFacts, error) {
	out := map[string]store.LibraryItemFacts{}
	for _, id := range ids {
		if facts, ok := f[id]; ok {
			out[id] = facts
		}
	}
	return out, nil
}

// Title search answers from the Library only (#2151 step 16): its titles come back under their
// library item ids with their facts, and a term the Library has no title for is no results, not
// an error.
func TestLibrarySearchAnswersFromTheLibrary(t *testing.T) {
	ctx := context.Background()
	titles := titleSearchFake{
		{ID: "lib-film", Kind: "movie", Name: "Harbor Lights", TMDBID: "90000001", Rating: "PG", SizeBytes: 4_000},
		{ID: "lib-show", Kind: "series", Name: "Harbor Watch", TVDBID: "90000002", Rating: "TV-Y"},
	}
	facts := itemFactsFake{"lib-film": {Year: 1999, Overview: "A lighthouse.", Genres: []string{"Drama"}, RuntimeMs: 95 * 60_000}}

	got, err := librarySearch{titles: titles, facts: facts}.Search(ctx, "harbor", 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("search = %+v, %v; want the Library's two titles", got, err)
	}
	if film := got[0]; film.LibraryItemID != "lib-film" || film.TMDBID != 90000001 || film.Year != 1999 ||
		film.RuntimeMinutes != 95 || film.Overview != "A lighthouse." || film.OfficialRating != "PG" || film.SizeBytes != 4_000 {
		t.Errorf("film = %+v; want it under its library item id with its facts", film)
	}
	if show := got[1]; show.LibraryItemID != "lib-show" || show.MediaType != library.Series || show.TVDBID != 90000002 {
		t.Errorf("show = %+v", show)
	}
	if got, err := (librarySearch{titles: titleSearchFake{}, facts: facts}).Search(ctx, "nothing", 10); err != nil || len(got) != 0 {
		t.Errorf("no match = %+v, %v; want no results and no error", got, err)
	}
}
