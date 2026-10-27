package ideas_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/ideas"
	"github.com/loomarr/loomarr/internal/provision"
)

var now = time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)

// The helpers take a small n and key the title at 90,000,000+n, above every real provider id
// (#1592).
func movie(n int, name string, year int, genres ...string) ideas.Item {
	return ideas.Item{Keys: []provision.Key{provision.Key(fmt.Sprintf("movie:tmdb:%d", 90_000_000+n))},
		MediaType: provision.Movie, Name: name, Year: year, Genres: genres}
}

func series(n int, name string, year int, genres ...string) ideas.Item {
	id := 90_000_000 + n
	return ideas.Item{Keys: []provision.Key{provision.Key(fmt.Sprintf("series:tvdb:%d", id)), provision.Key(fmt.Sprintf("series:tmdb:%d", id))},
		MediaType: provision.Series, Name: name, Year: year, Genres: genres}
}

func byID(list []ideas.Idea) map[string]ideas.Idea {
	out := map[string]ideas.Idea{}
	for _, idea := range list {
		out[idea.ID] = idea
	}
	return out
}

// A genre with enough titles that no channel plays yet is an idea, counted by what's unaired. A
// title is on a channel when ANY of its keys is: a series added by its TMDB key still counts.
func TestBuild_GenreIdeaCountsOnlyUnairedTitles(t *testing.T) {
	var lib []ideas.Item
	for i := 1; i <= 7; i++ {
		lib = append(lib, movie(i, fmt.Sprintf("Film %d", i), 1990+i, "Comedy"))
	}
	lib = append(lib, series(100, "Show", 2001, "comedy"), series(101, "Other show", 2002, "Comedy"))
	onChannel := map[provision.Key]bool{"movie:tmdb:90000001": true, "series:tmdb:90000101": true}

	got := byID(ideas.Build(ideas.Input{Library: lib, OnChannel: onChannel, Now: now}))

	idea, ok := got["genre:comedy"]
	if !ok {
		t.Fatalf("no comedy idea in %v", got)
	}
	if idea.Reason.Kind != ideas.ReasonUnaired || idea.Reason.Count != 7 {
		t.Errorf("reason = %+v, want unaired 7 (6 films + 1 series; one of each is on a channel)", idea.Reason)
	}
	if idea.Movies != 6 || idea.Series != 1 || len(idea.Keys) != 7 {
		t.Errorf("idea = %d films, %d series, %d keys; want 6, 1, 7", idea.Movies, idea.Series, len(idea.Keys))
	}
	if idea.Facet != ideas.FacetGenre || idea.Value != "Comedy" {
		t.Errorf("facet = %s %q, want genre as the library first spells it", idea.Facet, idea.Value)
	}
	for _, k := range idea.Keys {
		if k == "movie:tmdb:90000001" || k == "series:tvdb:90000101" {
			t.Errorf("idea offers %s, which is already on a channel", k)
		}
	}
}

// Too few unaired titles is not a channel: no idea.
func TestBuild_SmallFacetIsNoIdea(t *testing.T) {
	var lib []ideas.Item
	for i := 1; i <= 5; i++ {
		lib = append(lib, movie(i, fmt.Sprintf("Film %d", i), 0, "Western"))
	}
	if got := ideas.Build(ideas.Input{Library: lib, Now: now}); len(got) != 0 {
		t.Errorf("five westerns made ideas %+v, want none", got)
	}
}

// Decades group movies and series by release year; an item with no year joins no decade.
func TestBuild_DecadeIdea(t *testing.T) {
	var lib []ideas.Item
	for i := 0; i < 6; i++ {
		lib = append(lib, movie(i+1, fmt.Sprintf("Film %d", i), 1980+i))
	}
	lib = append(lib, movie(50, "Undated", 0))
	idea, ok := byID(ideas.Build(ideas.Input{Library: lib, Now: now}))["decade:1980"]
	if !ok || idea.Reason.Count != 6 || idea.Value != "1980" {
		t.Fatalf("decade idea = %+v (ok %v), want 1980 with 6 titles", idea, ok)
	}
}

// A holiday ahead makes an idea from titles that fit it (§6 floor: the holiday's name, or a mood
// word the genres back up — never a genre or a mood word alone), whether or not they already air.
func TestBuild_HolidayAheadMatchesTitlesNotGenres(t *testing.T) {
	lib := []ideas.Item{
		movie(1, "A Halloween Party", 2001),
		movie(2, "The Haunted Manor", 2003, "Horror"),
		series(3, "Spooky Tales", 1999, "Animation", "Fantasy"),
		movie(4, "Quiet Evening", 2005, "Horror"),                   // horror genre, not about the holiday
		movie(5, "Ghost Mall", 2001, "Comedy", "Drama"),             // a mood word alone
		movie(6, "Agent Zero: Ghost Directive", 2011, "Action"),     // a mood word alone
		movie(7, "Phantom Heart Ghost", 1990, "Fantasy", "Romance"), // fantasy without a family genre
	}
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	got := ideas.Build(ideas.Input{
		Library: lib, OnChannel: map[provision.Key]bool{"movie:tmdb:90000002": true}, Now: now,
		Holidays: []ideas.Holiday{{ID: "halloween", Start: start, End: end}}, Vocabulary: holidayvocab.Default(),
	})
	if len(got) == 0 || got[0].ID != "holiday:halloween" {
		t.Fatalf("ideas = %+v, want the holiday first", got)
	}
	idea := got[0]
	if idea.Reason.Kind != ideas.ReasonHoliday || idea.Reason.HolidayID != "halloween" ||
		!idea.Reason.StartsAt.Equal(start) || !idea.Reason.EndsAt.Equal(end) {
		t.Errorf("reason = %+v, want the holiday and its window", idea.Reason)
	}
	if idea.Reason.Count != 3 || idea.Movies != 2 || idea.Series != 1 {
		t.Errorf("holiday idea = %+v, want 3 titles (2 films, 1 series), the horror-genre film and bare ghost titles excluded", idea)
	}
}

// With no model to judge, the floor alone decides; too few fitting titles means no holiday idea.
func TestBuild_HolidayIdeaNeedsMinimumFittingTitles(t *testing.T) {
	lib := []ideas.Item{
		movie(1, "A Halloween Party", 2001),
		movie(2, "Ghost Mall", 2001, "Comedy"),
		movie(3, "Ghost Directive", 2011, "Action"),
		movie(4, "The Hollow Witch", 2015, "Horror"),
	}
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	got := ideas.Build(ideas.Input{Library: lib, Now: now,
		Holidays: []ideas.Holiday{{ID: "halloween", Start: start, End: start.AddDate(0, 1, 0)}}, Vocabulary: holidayvocab.Default()})
	if _, ok := byID(got)["holiday:halloween"]; ok {
		t.Fatalf("ideas = %+v, want no Halloween idea from two fitting titles", got)
	}
}

// Ranking is stable: holidays soonest first, then facets by unaired count, then id.
func TestBuild_OrderIsDeterministic(t *testing.T) {
	var lib []ideas.Item
	for i := 1; i <= 9; i++ {
		lib = append(lib, movie(i, fmt.Sprintf("Film %d", i), 0, "Drama"))
	}
	for i := 10; i <= 16; i++ {
		lib = append(lib, movie(i, fmt.Sprintf("Film %d", i), 0, "Action"))
	}
	for i := 20; i <= 26; i++ {
		lib = append(lib, movie(i, fmt.Sprintf("Film %d", i), 0, "Anime"))
	}
	var ids []string
	for _, idea := range ideas.Build(ideas.Input{Library: lib, Now: now}) {
		ids = append(ids, idea.ID)
	}
	want := []string{"genre:drama", "genre:action", "genre:anime"}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Errorf("order = %v, want %v", ids, want)
	}
}

// A person's hidden ideas stay hidden.
func TestBuild_HiddenIdeasAreLeftOut(t *testing.T) {
	var lib []ideas.Item
	for i := 1; i <= 6; i++ {
		lib = append(lib, movie(i, fmt.Sprintf("Film %d", i), 0, "Drama"))
	}
	got := ideas.Build(ideas.Input{Library: lib, Now: now, Hidden: map[string]bool{"genre:drama": true}})
	if len(got) != 0 {
		t.Errorf("hidden idea came back: %+v", got)
	}
}
