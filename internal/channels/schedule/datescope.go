package schedule

// An era is not a second date scope (#1817, #1877): an era range R filters exactly what
// Dates{MovieRelease, SeriesPremiere, SeriesAiring = [R]} filters — a movie by its release year,
// a series by its premiere year, and each episode by its own airing year, with an unknown year (0)
// failing open on every axis. The scope's `era` alias was folded into stored dates by migration
// 00230 (#2147) and is retired; EraDates and DateScope.Era are how code still speaks in eras.

// EraDates is the date scope an era range stands for: the same window on all three
// axes. The any-era range constrains nothing and returns nil.
func EraDates(r Range) *DateScope {
	if r == (Range{}) {
		return nil
	}
	return &DateScope{MovieRelease: []Range{r}, SeriesPremiere: []Range{r}, SeriesAiring: []Range{r}}
}

// Validate applies the stored-policy rules for a date scope: years in 1900–2099, at most
// one open end per range, and each axis sorted and non-overlapping.
func (d *DateScope) Validate() error {
	return validateDates("dates", d)
}

// Era returns the single range when the scope is era-shaped (all three axes hold the same
// one range), which is exactly the set of scopes the era shortcut can express.
func (d *DateScope) Era() (Range, bool) {
	if d == nil || len(d.MovieRelease) != 1 || !sameRanges(d.MovieRelease, d.SeriesPremiere) || !sameRanges(d.MovieRelease, d.SeriesAiring) {
		return Range{}, false
	}
	return d.MovieRelease[0], true
}

func datesEqual(a, b *DateScope) bool {
	if a == nil || b == nil {
		return a == b
	}
	return sameRanges(a.MovieRelease, b.MovieRelease) && sameRanges(a.SeriesPremiere, b.SeriesPremiere) &&
		sameRanges(a.SeriesAiring, b.SeriesAiring)
}

// FillerEra is the filler era a channel's date scope implies (§10): a single window as
// an era range (which may be open-ended, as the era shortcut always could), or a
// disjoint union as windows. Both nil when the scope has no dates.
func (s ScopePolicy) FillerEra() (*Range, []Range) {
	windows := s.FillerEraWindows()
	switch len(windows) {
	case 0:
		return nil, nil
	case 1:
		return &windows[0], nil
	default:
		return nil, windows
	}
}
