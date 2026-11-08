package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
)

type fakeLibraryLister struct {
	items []library.SearchResult
	calls int
}

func (f *fakeLibraryLister) AllItems(context.Context) ([]library.SearchResult, error) {
	f.calls++
	return f.items, nil
}

// Channel ideas (#1665) read the whole library, and Home asks on every visit, so one listing
// serves every request inside the TTL. An item answers to every key a lineup entry could carry.
func TestLibraryIdeaItemsMapsKeysAndCaches(t *testing.T) {
	lister := &fakeLibraryLister{items: []library.SearchResult{
		{MediaType: library.Series, TVDBID: 90000007, TMDBID: 90000070, Name: "A show", Year: 2001, Genres: []string{"Comedy"}},
		{MediaType: library.Movie, TMDBID: 90000008, Name: "A film", Year: 1999},
		{MediaType: library.Movie, Name: "No provider id"},
	}}
	now := time.Unix(1_800_000_000, 0)
	a := &libraryIdeaItems{lib: lister, now: func() time.Time { return now }}

	items, err := a.IdeaItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, fmt.Sprintf("%s %v %d %v", it.MediaType, it.Keys, it.Year, it.Genres))
	}
	want := "[series [series:tvdb:90000007 series:tmdb:90000070] 2001 [Comedy] movie [movie:tmdb:90000008] 1999 []]"
	if fmt.Sprint(got) != want {
		t.Errorf("items = %v\nwant    %s", got, want)
	}

	now = now.Add(ideaLibraryTTL - time.Second)
	_, _ = a.IdeaItems(context.Background())
	if lister.calls != 1 {
		t.Errorf("library listed %d times inside the TTL, want 1", lister.calls)
	}
	now = now.Add(2 * time.Second)
	_, _ = a.IdeaItems(context.Background())
	if lister.calls != 2 {
		t.Errorf("library listed %d times after the TTL, want 2", lister.calls)
	}
}

type ideaFactsFake map[string]store.LibraryItemFacts

func (f ideaFactsFake) LibraryItemFacts(_ context.Context, ids []string) (map[string]store.LibraryItemFacts, error) {
	out := map[string]store.LibraryItemFacts{}
	for _, id := range ids {
		if facts, ok := f[id]; ok {
			out[id] = facts
		}
	}
	return out, nil
}

// Channel ideas draw from the Library's titles with their year and genres (#2151 step 16).
func TestLibraryIdeaTitlesReadTheLibrary(t *testing.T) {
	facts := ideaFactsFake{"lib-film": {Year: 1999, Genres: []string{"Drama"}}, "lib-show": {Year: 2001}}
	got, err := libraryIdeaTitles{titles: testLibraryTitles, facts: facts}.AllItems(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("titles = %+v, %v; want the Library's two", got, err)
	}
	if film := got[0]; film.LibraryItemID != "lib-film" || film.Year != 1999 || len(film.Genres) != 1 || film.Genres[0] != "Drama" {
		t.Errorf("film = %+v; want its Library year and genres", film)
	}
	if got[1].LibraryItemID != "lib-show" || got[1].Year != 2001 {
		t.Errorf("titles = %+v", got)
	}
}
