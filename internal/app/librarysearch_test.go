package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
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

// Title search asks the Library first (#2151 step 15e): its titles come back under their library
// item ids with their facts, and the media server adds only titles the Library didn't answer. With
// no media server, a term the Library has no title for is no results, not an error.
func TestLibrarySearchAsksTheLibraryFirst(t *testing.T) {
	ctx := context.Background()
	titles := titleSearchFake{
		{ID: "lib-film", Kind: "movie", Name: "Harbor Lights", TMDBID: "90000001", Rating: "PG", SizeBytes: 4_000},
		{ID: "lib-show", Kind: "series", Name: "Harbor Watch", TVDBID: "90000002", Rating: "TV-Y"},
	}
	facts := itemFactsFake{"lib-film": {Year: 1999, Overview: "A lighthouse.", Genres: []string{"Drama"}, RuntimeMs: 95 * 60_000}}

	server := testkit.NewMediaServer(t)
	t.Cleanup(server.Close)
	server.SearchItems = []testkit.SearchStub{
		{LibraryItemID: "emby-1", Name: "Harbor Lights", Type: "Movie", TMDBID: 90000001, Terms: []string{"harbor"}}, // the Library holds it
		{LibraryItemID: "emby-9", Name: "Harbor Nights", Type: "Movie", TMDBID: 90000009, Terms: []string{"harbor"}},
	}
	got, err := librarySearch{titles: titles, facts: facts, lib: newTestLibraryClient(server)}.Search(ctx, "harbor", 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("search = %+v, %v; want the Library's two titles and the server's other one", got, err)
	}
	if film := got[0]; film.LibraryItemID != "lib-film" || film.TMDBID != 90000001 || film.Year != 1999 ||
		film.RuntimeMinutes != 95 || film.Overview != "A lighthouse." || film.OfficialRating != "PG" || film.SizeBytes != 4_000 {
		t.Errorf("film = %+v; want it under its library item id with its facts", film)
	}
	if show := got[1]; show.LibraryItemID != "lib-show" || show.MediaType != library.Series || show.TVDBID != 90000002 {
		t.Errorf("show = %+v", show)
	}
	if got[2].LibraryItemID != "emby-9" {
		t.Errorf("third = %+v; want the server's title the Library doesn't hold", got[2])
	}

	// A Library that fills the limit asks the media server nothing.
	if full, err := (librarySearch{titles: titles, facts: facts, lib: newTestLibraryClient(server)}).Search(ctx, "harbor", 2); err != nil ||
		len(full) != 2 || full[1].LibraryItemID != "lib-show" {
		t.Errorf("a full Library page = %+v, %v", full, err)
	}

	none := library.NewDynamic(func() library.Connection { return library.Connection{} }, "device-1")
	if got, err := (librarySearch{titles: titleSearchFake{}, facts: facts, lib: none}).Search(ctx, "nothing", 10); err != nil || len(got) != 0 {
		t.Errorf("no match and no media server = %+v, %v; want no results and no error", got, err)
	}
}
