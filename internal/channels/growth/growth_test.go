package growth_test

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/rating"
)

func sitcom(key string, year int, r rating.Rating, genres ...string) growth.Title {
	return growth.Title{Key: provision.Key("series:tvdb:" + key), Kind: growth.Series, Year: year, Rating: r, Genres: genres, Airable: true}
}

func film(key string, year int, r rating.Rating, franchise int, genres ...string) growth.Title {
	return growth.Title{Key: provision.Key("movie:tmdb:" + key), Kind: growth.Film, Year: year, Rating: r, Franchise: franchise, Genres: genres, Airable: true}
}

// Eight family sitcoms from 1987 to 2002 draw a line around family sitcoms of those years: the
// install's channel whose stored scope ("Comedy, Family", no years) admitted 163 series, 73 TV-MA.
func TestTheSeedsDrawTheLine(t *testing.T) {
	var seeds []growth.Title
	for i, y := range []int{1987, 1989, 1990, 1993, 1994, 1997, 1999, 2002} {
		seeds = append(seeds, sitcom(string(rune('a'+i)), y, rating.Rating("TV-PG"), "Comedy", "Family"))
	}
	l := growth.DrawLine(seeds, 3)
	if l.Fixed || !slices.Equal(l.Kinds, []growth.Kind{growth.Series}) || l.YearFrom > 1990 || l.YearTo < 1999 ||
		l.MaxRating != "TV-PG" || !slices.Equal(l.Genres, []string{"comedy", "family"}) || len(l.Odd) != 0 {
		t.Fatalf("line %+v", l)
	}
	for _, tc := range []struct {
		name string
		t    growth.Title
		want growth.Fit
	}{
		{"a family sitcom of the years", sitcom("x1", 1995, "TV-G", "Comedy", "Family"), growth.Clear},
		{"a comedy of the years without family", sitcom("x2", 1995, "TV-PG", "Comedy"), growth.Unclear},
		{"an adult comedy", sitcom("x3", 1995, "TV-MA", "Comedy", "Family"), growth.Outside},
		{"a family sitcom from 2015", sitcom("x4", 2015, "TV-PG", "Comedy", "Family"), growth.Outside},
		{"a drama", sitcom("x5", 1995, "TV-PG", "Drama"), growth.Outside},
		{"a family film", film("x6", 1995, "PG", 0, "Comedy", "Family"), growth.Outside},
		{"an unrated family sitcom", sitcom("x7", 1995, "", "Comedy", "Family"), growth.Unrated},
		{"a family sitcom with no year", sitcom("x8", 0, "TV-PG", "Comedy", "Family"), growth.Unclear},
	} {
		if got := l.Classify(tc.t, 1); got.Fit != tc.want {
			t.Errorf("%s: %+v, want %s", tc.name, got, tc.want)
		}
	}
}

// An odd seed keeps airing but does not stretch the line (decision 5); a seed with nothing airable
// is ignored.
func TestAnOddSeedDoesNotStretchTheLine(t *testing.T) {
	seeds := []growth.Title{
		sitcom("a", 1988, "TV-PG", "Comedy"), sitcom("b", 1990, "TV-PG", "Comedy"),
		sitcom("c", 1992, "TV-PG", "Comedy"), sitcom("d", 1994, "TV-PG", "Comedy"),
		sitcom("odd", 2019, "TV-MA", "Comedy"),
	}
	gone := sitcom("gone", 1950, "TV-G", "Western")
	gone.Airable = false
	l := growth.DrawLine(append(seeds, gone), 3)
	if len(l.Odd) != 1 || l.Odd[0].Key != "series:tvdb:odd" || !slices.Equal(l.Odd[0].Reasons, []string{"years"}) {
		t.Fatalf("odd seeds %+v, want the 2019 show on years", l.Odd)
	}
	if l.YearTo >= 2019 || l.MaxRating != "TV-PG" || l.YearFrom <= 1950 {
		t.Fatalf("the odd seed or the unairable one stretched the line: %+v", l)
	}
}

// Narrow channels stay narrow (decision 6): one seed never grows, and an all-franchise channel's
// line is the franchise.
func TestNarrowChannelsStayNarrow(t *testing.T) {
	one := growth.DrawLine([]growth.Title{sitcom("a", 1990, "TV-PG", "Comedy")}, 3)
	if !one.Fixed || one.Classify(sitcom("b", 1990, "TV-PG", "Comedy"), 0).Fit != growth.Outside {
		t.Fatalf("a one-seed channel grew: %+v", one)
	}
	franchise := growth.DrawLine([]growth.Title{film("a", 1977, "PG", 90000010, "Sci-Fi"), film("b", 1980, "PG", 90000010, "Sci-Fi")}, 3)
	if got := franchise.Classify(film("c", 1983, "PG", 90000010, "Sci-Fi"), 1); got.Fit != growth.Clear {
		t.Errorf("a film of the franchise: %+v", got)
	}
	if got := franchise.Classify(film("d", 1979, "PG", 0, "Sci-Fi"), 1); got.Fit != growth.Outside {
		t.Errorf("a sci-fi film outside the franchise: %+v", got)
	}
}

// The target is a week of episodes without a repeat, enough shows that none dominates a day, and
// a month's rest for every film (decisions 2 and 4).
func TestTheTargetIsAWeekAndAMonth(t *testing.T) {
	got := growth.TargetFor(growth.Need{DailyAirtime: 20 * time.Hour, MeanEpisode: 22 * time.Minute}, 0.15)
	if got.Episodes != 382 || got.Shows != 7 || got.Films != 0 {
		t.Errorf("series target %+v, want 382 episodes, 7 shows", got)
	}
	got = growth.TargetFor(growth.Need{DailyAirtime: 20 * time.Hour, FilmShare: 1, MeanFilm: 100 * time.Minute}, 0.15)
	if got.Films != 360 || got.Episodes != 0 {
		t.Errorf("film target %+v, want 360 films", got)
	}
}
