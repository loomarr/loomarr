package growth

import (
	"sort"

	"github.com/loomarr/loomarr/internal/library/provision"
)

// Candidate is a library title the channel does not carry, with how much it would add.
type Candidate struct {
	Title    Title
	Episodes int // a series' episodes in the library; 0 for a film
}

// Carried is what a channel holds now.
type Carried struct {
	Episodes, Shows, Films int
}

// Pick is a title the fill adds, and its fit.
type Pick struct {
	Key provision.Key
	Fit Fit
}

// Judgement is the model's answer on a title for a channel's current line (decision 32); empty
// when it has not judged the title against that line.
type Judgement string

// The model's fixed choices.
const (
	JudgedFits   Judgement = "fits"
	JudgedNotFit Judgement = "does_not_fit"
)

// Plan chooses what joins the channel tonight (decision 8): clear fits first, the closest to the
// seeds first, until the channel reaches its target for that kind; then unclear fits, while the
// channel is still short. A title the model judged does not fit never joins. An unclear fit joins
// only once the model judged it fits, or, with withUnclear, on an install with no model, where
// nothing will ever review it. Nothing joins a fixed line, and nothing a person declined.
func Plan(l Line, candidates []Candidate, carried Carried, target Target, bar float64, declined func(provision.Key) bool,
	judged func(provision.Key) Judgement, withUnclear bool) []Pick {
	if l.Fixed {
		return nil
	}
	type scored struct {
		Candidate
		fit   Fit
		share float64
		dist  int
	}
	mid := (l.YearFrom + l.YearTo) / 2
	var pool []scored
	for _, c := range candidates {
		if declined != nil && declined(c.Title.Key) {
			continue
		}
		var j Judgement
		if judged != nil {
			j = judged(c.Title.Key)
		}
		v := l.Classify(c.Title, bar)
		if j == JudgedNotFit || (v.Fit != Clear && (v.Fit != Unclear || (!withUnclear && j != JudgedFits))) {
			continue
		}
		s := scored{Candidate: c, fit: v.Fit, share: 1}
		if len(l.Genres) > 0 {
			s.share = float64(overlap(c.Title.Genres, l.Genres)) / float64(len(l.Genres))
		}
		if l.YearTo != 0 && c.Title.Year != 0 {
			s.dist = abs(c.Title.Year - mid)
		}
		pool = append(pool, s)
	}
	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		if a.fit != b.fit {
			return a.fit == Clear
		}
		if a.share != b.share {
			return a.share > b.share
		}
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		return a.Title.Key < b.Title.Key
	})

	needEpisodes, needShows, needFilms := target.Episodes-carried.Episodes, target.Shows-carried.Shows, target.Films-carried.Films
	var picks []Pick
	for _, s := range pool {
		switch s.Title.Kind {
		case Series:
			if needEpisodes <= 0 && needShows <= 0 {
				continue
			}
			needEpisodes -= s.Episodes
			needShows--
		case Film:
			if needFilms <= 0 {
				continue
			}
			needFilms--
		default:
			continue
		}
		picks = append(picks, Pick{Key: s.Title.Key, Fit: s.fit})
	}
	return picks
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
