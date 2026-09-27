package schedule_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// productionGenreEntry is one lineup entry exactly as #1630's household channel stored it: an
// in-library series approved as a complete run, its genres as the MEDIA SERVER names them.
func productionGenreEntry(key string, year int, genres ...string) schedule.LineupEntry {
	return schedule.LineupEntry{
		Key: provision.Key(key), Title: "Series " + key, Year: year, OfficialRating: "TV-PG",
		Genres: genres, EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeComplete},
	}
}

func genreChannelAvail(keys ...string) schedule.Availability {
	episodes := make(map[string][]schedule.ResolvedProgram, len(keys))
	for _, key := range keys {
		episodes[key] = []schedule.ResolvedProgram{
			{LibraryItemID: key + "-e1", Title: "One", DurationMs: 2_700_000, Season: 1, Episode: 1},
			{LibraryItemID: key + "-e2", Title: "Two", DurationMs: 2_700_000, Season: 1, Episode: 2},
		}
	}
	return newSeriesAvail(episodes)
}

// #1630, from the household install's own rows: four available series and a genre scope written
// in TMDB's TV vocabulary ("Sci-Fi & Fantasy", "Action & Adventure"). The media server tags the
// same shows "Science Fiction", "Action", "Adventure", so an exact-name match excluded every entry
// as out_of_scope: desired `[]`, no pending slot, status `empty`, and a channel that never aired.
func TestComputeDesired_TVCompoundGenreScopeAdmitsMediaServerGenres(t *testing.T) {
	lineup := []schedule.LineupEntry{
		productionGenreEntry("series:tvdb:1", 1987, "Action", "Adventure", "Drama", "Mystery", "Science Fiction"),
		productionGenreEntry("series:tvdb:2", 1993, "Action", "Adventure", "Drama", "Science Fiction"),
		productionGenreEntry("series:tvdb:3", 1995, "Action", "Adventure", "Science Fiction"),
		productionGenreEntry("series:tvdb:4", 2022, "Action", "Adventure", "Drama", "Science Fiction"),
	}
	policy := schedule.ChannelPolicy{
		Scope: schedule.ScopePolicy{Genres: schedule.GenreFilter{
			Include: []string{"Sci-Fi & Fantasy", "Action & Adventure", "Documentary"},
		}},
		Ordering: schedule.OrderSyndication,
	}
	avail := genreChannelAvail("series:tvdb:1", "series:tvdb:2", "series:tvdb:3", "series:tvdb:4")

	got := programKeys(schedule.ComputeDesiredAt(seqChannel(), lineup, avail, schedule.PodFill, policy, time.Time{}))
	for _, e := range lineup {
		if !hasKey(got, e.Key) {
			t.Fatalf("%s aired no program; a TV compound genre must admit the genres it is made of (aired %v)", e.Key, got)
		}
	}
}

// The same rule in every direction a household can meet it: a TMDB-sourced entry carries the
// compound while a hand-edited scope names a part, and an exclude is the same comparison. A
// genre that no term covers still stays out, so the scope keeps narrowing.
func TestComputeDesired_GenreScopeMatchesTVCompoundsBothWays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		genres []string
		filter schedule.GenreFilter
		airs   bool
	}{
		{"include part admits compound entry", []string{"Sci-Fi & Fantasy"}, schedule.GenreFilter{Include: []string{"Science Fiction"}}, true},
		{"include compound admits part entry", []string{"Adventure"}, schedule.GenreFilter{Include: []string{"Action & Adventure"}}, true},
		{"include war compound admits part entry", []string{"War"}, schedule.GenreFilter{Include: []string{"War & Politics"}}, true},
		{"exclude compound removes part entry", []string{"Fantasy", "Drama"}, schedule.GenreFilter{Exclude: []string{"Sci-Fi & Fantasy"}}, false},
		{"exclude part removes compound entry", []string{"Action & Adventure"}, schedule.GenreFilter{Exclude: []string{"Action"}}, false},
		{"uncovered genre stays out", []string{"Action", "Drama"}, schedule.GenreFilter{Include: []string{"Documentary"}}, false},
		{"parts never match each other", []string{"Fantasy"}, schedule.GenreFilter{Include: []string{"Science Fiction"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lineup := []schedule.LineupEntry{productionGenreEntry("series:tvdb:9", 2000, tc.genres...)}
			policy := schedule.ChannelPolicy{Scope: schedule.ScopePolicy{Genres: tc.filter}}
			got := programKeys(schedule.ComputeDesiredAt(seqChannel(), lineup, genreChannelAvail("series:tvdb:9"), schedule.PodFill, policy, time.Time{}))
			if airs := hasKey(got, "series:tvdb:9"); airs != tc.airs {
				t.Fatalf("entry genres %v under %+v: airs = %v, want %v", tc.genres, tc.filter, airs, tc.airs)
			}
		})
	}
}
