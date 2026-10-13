package schedule

import (
	"encoding/json"
	"errors"
)

// Era is a shortcut for Dates, not a second date scope (#1817, #1877). An era range R
// filters exactly what Dates{MovieRelease, SeriesPremiere, SeriesAiring = [R]} filters:
// a movie by its release year, a series by its premiere year, and each episode by its
// own airing year, with an unknown year (0) failing open on every axis.
//
// The alias is resolved on DECODE, so a policy saved with `era` keeps its lineup with no
// one-shot migration (a goose migration runs once and cannot follow later writers), and
// the backend never writes `era` again. ScopePolicy has no Era field, so nothing can set
// one that the scheduler would then ignore.

// ErrEraAndDates is the ambiguous-request error: `era` is a shortcut for `dates`, so a
// request carrying both does not say which one it means.
var ErrEraAndDates = errors.New("send either era or dates, not both: era is a shortcut for the same year range on movie release, series premiere and episode airing")

// UnmarshalJSON reads a scope, folding the retired `era` alias into Dates.
//
// A stored policy may carry both (the two used to be ANDed), so both decode to their
// intersection rather than failing: a saved channel must stay readable. The API rejects
// the same shape on a request through ChannelPolicy.DateAliasConflict.
func (s *ScopePolicy) UnmarshalJSON(b []byte) error {
	type plain ScopePolicy // no methods: decodes without recursing into this one
	var in struct {
		plain
		Era *Range `json:"era"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*s = ScopePolicy(in.plain)
	if in.Era == nil || *in.Era == (Range{}) {
		return nil // absent, or the any-era range: no constraint
	}
	s.eraAndDates = s.Dates != nil
	s.Dates = withEra(s.Dates, *in.Era)
	return nil
}

// DateAliasConflict reports a decoded request that carried both `era` and `dates`, on the
// channel scope or any rule's scope. Call it on a REQUEST body only: a stored policy with
// both is legal legacy data and has already been folded to their intersection.
func (p ChannelPolicy) DateAliasConflict() error {
	if p.Scope.eraAndDates {
		return ErrEraAndDates
	}
	for _, r := range p.Rules {
		if r.What != nil && r.What.eraAndDates {
			return ErrEraAndDates
		}
	}
	return nil
}

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

// withEra ANDs an era range into a date scope, axis by axis. An axis with no ranges was
// unconstrained, so it becomes [era]; otherwise each range is clipped to the era.
//
// ⚠ An axis whose ranges miss the era entirely cannot be represented: an empty list
// means "unconstrained", not "nothing". That stored shape aired no dated title on the
// axis, which is the conflict the old editor warned about; the era wins there, as the
// operator-facing field.
func withEra(d *DateScope, era Range) *DateScope {
	if d == nil {
		return EraDates(era)
	}
	clip := func(ranges []Range) []Range {
		if len(ranges) == 0 {
			return []Range{era}
		}
		var out []Range
		for _, r := range ranges {
			if both, ok := intersectRange(r, era); ok {
				out = append(out, both)
			}
		}
		if len(out) == 0 {
			return []Range{era}
		}
		return NormalizeRanges(out)
	}
	return &DateScope{
		MovieRelease:   clip(d.MovieRelease),
		SeriesPremiere: clip(d.SeriesPremiere),
		SeriesAiring:   clip(d.SeriesAiring),
	}
}

// intersectRange is the overlap of two ranges whose 0 bounds are open.
func intersectRange(a, b Range) (Range, bool) {
	out := Range{From: max(a.From, b.From)}
	switch {
	case a.To == 0:
		out.To = b.To
	case b.To == 0:
		out.To = a.To
	default:
		out.To = min(a.To, b.To)
	}
	return out, out.To == 0 || out.From <= out.To
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
