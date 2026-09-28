package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
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
		{MediaType: library.Series, TVDBID: 7, TMDBID: 70, Name: "A show", Year: 2001, Genres: []string{"Comedy"}},
		{MediaType: library.Movie, TMDBID: 8, Name: "A film", Year: 1999},
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
	want := "[series [series:tvdb:7 series:tmdb:70] 2001 [Comedy] movie [movie:tmdb:8] 1999 []]"
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
