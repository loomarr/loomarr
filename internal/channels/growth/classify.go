package growth

import "slices"

// Fit is how a library title sits against a channel's line (decision 8).
type Fit string

const (
	// Clear joins at the next midnight; the model reviews it afterwards.
	Clear Fit = "clear"
	// Unclear waits for the model's verdict; with no model it joins only while the channel is short.
	Unclear Fit = "unclear"
	// Unrated never joins by itself; it is listed in the shortfall as "add by hand" (decision 29).
	Unrated Fit = "unrated"
	// Outside does not join.
	Outside Fit = "outside"
)

// Verdict is a title's fit and, for anything but a clear fit, why.
type Verdict struct {
	Fit    Fit
	Reason string // "fixed", "franchise", "kind", "rating", "years", "genres", "year unknown", "few genres"
}

// Classify sits a library title against the line. bar is the share a clear fit reaches both ways:
// of the line's genres the title carries, and of the title's genres the line holds. Below it the
// title is unclear rather than clear; the simulator sets it (#4140).
func (l Line) Classify(t Title, bar float64) Verdict {
	switch {
	case l.Fixed:
		return Verdict{Outside, "fixed"}
	case l.Franchise > 0:
		if t.Franchise == l.Franchise {
			return Verdict{Fit: Clear}
		}
		return Verdict{Outside, "franchise"}
	case !slices.Contains(l.Kinds, t.Kind):
		return Verdict{Outside, "kind"}
	}
	rank, rated := lineRank(t.Rating)
	if !rated {
		return Verdict{Unrated, "unrated"}
	}
	if max, ok := lineRank(l.MaxRating); ok && rank > max {
		return Verdict{Outside, "rating"}
	}
	if l.YearTo != 0 && t.Year != 0 && (t.Year < l.YearFrom || t.Year > l.YearTo) {
		return Verdict{Outside, "years"}
	}
	share := 1.0
	if len(l.Genres) > 0 {
		n := overlap(t.Genres, l.Genres)
		if n == 0 {
			return Verdict{Outside, "genres"}
		}
		share = float64(n) / float64(len(l.Genres))
		// The line must also be most of what the title is: a crime thriller tagged drama carries a
		// drama line's genre, but is not a drama, and a clear fit airs before the model sees it.
		if own := float64(overlap(l.Genres, t.Genres)) / float64(len(t.Genres)); own < share {
			share = own
		}
	}
	switch {
	case l.YearTo != 0 && t.Year == 0:
		return Verdict{Unclear, "year unknown"}
	case share < bar:
		return Verdict{Unclear, "few genres"}
	}
	return Verdict{Fit: Clear}
}
