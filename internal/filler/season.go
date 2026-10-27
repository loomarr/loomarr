package filler

import "github.com/loomarr/loomarr/internal/schedule"

// Holidays in a break (§10, #2030 decision 5). A clip is seasonal when a tag names a holiday with
// dates (Window.Season). Out of season it never airs, bookends included; in season at most half of
// a break's commercials are in-season clips, and at least one is when one is rested. Pins skip both
// rules, as they skip every matching rule, but an in-season pin counts toward the half.

// seasonOf reports whether c names a holiday with a season, and whether one it names is in season.
//
// ⚠ The ASSERTED tags, not Tags: Tags adds every rollup ancestor, so a `christmas` an operator
// parents under the `holiday` group would put every Christmas ad in season at Thanksgiving.
func seasonOf(c Clip, s schedule.HolidaySeason) (seasonal, inSeason bool) {
	for _, tag := range c.AssertedTags {
		if s.Dated[tag] {
			seasonal = true
			if s.InSeason[tag] {
				return true, true
			}
		}
	}
	return seasonal, false
}

// outOfSeason reports a clip that never airs at the window's instant.
func outOfSeason(c Clip, w Window) bool {
	seasonal, in := seasonOf(c, w.Season)
	return seasonal && !in
}

func inSeason(c Clip, w Window) bool {
	_, in := seasonOf(c, w.Season)
	return in
}

func filterSeason(clips []Clip, w Window) []Clip {
	out := make([]Clip, 0, len(clips))
	for _, c := range clips {
		if !outOfSeason(c, w) {
			out = append(out, c)
		}
	}
	return out
}

// seasonShare counts one break's commercials, and its in-season ones, against the half.
type seasonShare struct{ in, total int }

// admits reports whether c may join the break: a bookend or a year-round clip always may; an
// in-season commercial may while the in-season ones, it included, stay at most half of the
// commercials, it included (one always fits).
func (s seasonShare) admits(c Clip, w Window) bool {
	if c.EffectivePlacement() != PlacementBreakBody || !inSeason(c, w) {
		return true
	}
	return s.in < max(1, (s.total+1)/2)
}

func (s *seasonShare) add(c Clip, w Window) {
	if c.EffectivePlacement() != PlacementBreakBody {
		return
	}
	s.total++
	if inSeason(c, w) {
		s.in++
	}
}
