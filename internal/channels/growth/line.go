// Package growth decides what may join a channel by itself (#4140 step 2, #4333): the line its
// seeds draw, how a library title sits against that line, how big the channel should be, and the
// nightly fill that adds the titles that fit.
//
// The line, the classification, the target and the plan are pure: no store, no library, no model.
// Fill is the nightly job that reads the library and the channels, and adds what the plan chose
// through a proposal the binder unions onto the lineup. The channel's stored scope and audience
// ceiling apply first (schedule.Admitted); the line only narrows them, so growth can never admit a
// title the channel's limits refuse.
package growth

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/rating"
)

// Kind is what a title is: a series or a film.
type Kind string

const (
	Series Kind = "series"
	Film   Kind = "movie"
)

// Title is what growth reads of a seed or a library title.
type Title struct {
	Key    provision.Key
	Kind   Kind
	Year   int           // 0 = unknown
	Rating rating.Rating // unmapped = unrated
	Genres []string
	// Franchise is the TMDB collection a film belongs to; 0 = none or not known.
	Franchise int
	// Airable is whether the library holds something of it the channel can air. A seed that is
	// not airable is ignored when drawing the line (decision 5).
	Airable bool
}

// Line is what may join a channel by itself (decision 1).
type Line struct {
	// Fixed means nothing joins: a one-seed channel is a single-show channel (decision 6), and a
	// channel with no airable seed has nothing to draw from.
	Fixed bool
	// Franchise, when set, is the whole line: every seed belongs to that franchise (decision 6).
	Franchise int
	Kinds     []Kind
	// YearFrom and YearTo bound the years, margin included; both 0 when no seed has a year.
	YearFrom, YearTo int
	// MaxRating is the highest rating among the seeds that are not odd; empty when none is rated.
	MaxRating rating.Rating
	// Genres are the genres at least half the seeds share.
	Genres []string
	// Odd lists the seeds that do not match what most seeds share. They keep airing, are marked,
	// and do not stretch the line (decision 5).
	Odd []OddSeed
}

// Words is the line in plain words: what the model is asked against, and what a verdict is kept
// with, so a verdict given against an older line is asked again.
func (l Line) Words() string {
	switch {
	case l.Fixed:
		return "fixed"
	case l.Franchise > 0:
		return fmt.Sprintf("one franchise (collection %d)", l.Franchise)
	}
	kinds := make([]string, 0, len(l.Kinds))
	for _, k := range l.Kinds {
		if k == Film {
			kinds = append(kinds, "films")
		} else {
			kinds = append(kinds, "series")
		}
	}
	out := strings.Join(kinds, " and ")
	if l.YearTo != 0 {
		out += fmt.Sprintf(" from %d to %d", l.YearFrom, l.YearTo)
	}
	if l.MaxRating != "" {
		out += ", rated up to " + string(l.MaxRating)
	}
	if len(l.Genres) > 0 {
		out += ", " + strings.Join(l.Genres, " and ")
	}
	return out
}

// OddSeed is a seed outside what most seeds share, and how.
type OddSeed struct {
	Key     provision.Key
	Reasons []string // "kind", "years", "genres"
}

// DrawLine draws the line from a channel's seeds. margin is how many years either side of the
// seeds' span still fit ("a few either side").
func DrawLine(seeds []Title, margin int) Line {
	var airable []Title
	for _, s := range seeds {
		if s.Airable {
			airable = append(airable, s)
		}
	}
	if len(airable) <= 1 {
		return Line{Fixed: true}
	}
	if f := airable[0].Franchise; f > 0 && all(airable, func(s Title) bool { return s.Franchise == f }) {
		return Line{Franchise: f}
	}

	l := Line{Kinds: sharedKinds(airable), Genres: sharedGenres(airable)}
	if from, to, ok := coreYears(airable, margin); ok {
		l.YearFrom, l.YearTo = from-margin, to+margin
	}
	maxRank := -1
	for _, s := range airable {
		var why []string
		if !slices.Contains(l.Kinds, s.Kind) {
			why = append(why, "kind")
		}
		if s.Year != 0 && l.YearTo != 0 && (s.Year < l.YearFrom || s.Year > l.YearTo) {
			why = append(why, "years")
		}
		if len(l.Genres) > 0 && overlap(s.Genres, l.Genres) == 0 {
			why = append(why, "genres")
		}
		if len(why) > 0 {
			l.Odd = append(l.Odd, OddSeed{Key: s.Key, Reasons: why})
			continue
		}
		if r, ok := lineRank(s.Rating); ok && r > maxRank {
			maxRank, l.MaxRating = r, s.Rating
		}
	}
	return l
}

// lineRank is a rating's place for growth: the audience ladder's rank, with NC-17 a step above
// the R and TV-MA it shares the top rank with. The ladder keeps them together because a TV-MA
// ceiling must admit any adult film; a line is narrower than a ceiling, and a channel whose seeds
// stop at R has given no sign it airs NC-17.
func lineRank(r rating.Rating) (int, bool) {
	n, ok := r.Rank()
	n *= 2
	if r == "NC-17" {
		n++
	}
	return n, ok
}

// sharedKinds is every kind at least a quarter of the seeds are, and at least two: a mixed channel
// keeps both kinds, and one film among sitcoms does not open the channel to films.
func sharedKinds(seeds []Title) []Kind {
	count := map[Kind]int{}
	for _, s := range seeds {
		count[s.Kind]++
	}
	var out []Kind
	for _, k := range []Kind{Series, Film} {
		if n := count[k]; n >= 2 && 4*n >= len(seeds) {
			out = append(out, k)
		}
	}
	if len(out) == 0 { // every kind is held by a single seed: keep the commonest
		for _, k := range []Kind{Series, Film} {
			if count[k] > 0 && (len(out) == 0 || count[k] > count[out[0]]) {
				out = []Kind{k}
			}
		}
	}
	return out
}

// sharedGenres is every genre at least half the seeds carry, case-insensitively, sorted.
func sharedGenres(seeds []Title) []string {
	count := map[string]int{}
	for _, s := range seeds {
		seen := map[string]bool{}
		for _, g := range s.Genres {
			g = strings.ToLower(strings.TrimSpace(g))
			if g != "" && !seen[g] {
				seen[g] = true
				count[g]++
			}
		}
	}
	var out []string
	for g, n := range count {
		if 2*n >= len(seeds) {
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out
}

// coreYears is the span of the seeds' years that most seeds share, so one odd seed cannot stretch
// it. The years split into runs wherever two neighbours are further apart than both margins and a
// year between; the span is the run holding more than half the dated seeds, or every dated seed
// when no run does.
func coreYears(seeds []Title, margin int) (from, to int, ok bool) {
	var years []int
	for _, s := range seeds {
		if s.Year > 0 {
			years = append(years, s.Year)
		}
	}
	if len(years) == 0 {
		return 0, 0, false
	}
	sort.Ints(years)
	from, to = years[0], years[len(years)-1]
	start := 0
	for i := 1; i <= len(years); i++ {
		if i < len(years) && years[i]-years[i-1] <= 2*margin+1 {
			continue
		}
		if 2*(i-start) > len(years) {
			return years[start], years[i-1], true
		}
		start = i
	}
	return from, to, true
}

// overlap is how many of want the genres carry, case-insensitively.
func overlap(genres, want []string) int {
	n := 0
	for _, w := range want {
		if slices.ContainsFunc(genres, func(g string) bool { return strings.EqualFold(strings.TrimSpace(g), w) }) {
			n++
		}
	}
	return n
}

func all(ts []Title, pred func(Title) bool) bool {
	for _, t := range ts {
		if !pred(t) {
			return false
		}
	}
	return true
}
