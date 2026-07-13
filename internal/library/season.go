package library

import "fmt"

// SeasonPrecision controls what "in library" means for a series (§6, §15
// SEASON_PRECISION). Default `series`: the show existing is enough. `seasons`:
// the stricter opt-in that verifies each requested season is present before a
// series counts as available.
type SeasonPrecision string

const (
	PrecisionSeries  SeasonPrecision = "series"  // default
	PrecisionSeasons SeasonPrecision = "seasons" // stricter opt-in
)

// ParseSeasonPrecision validates the configured value (defaults to series).
func ParseSeasonPrecision(s string) (SeasonPrecision, error) {
	switch SeasonPrecision(s) {
	case "", PrecisionSeries:
		return PrecisionSeries, nil
	case PrecisionSeasons:
		return PrecisionSeasons, nil
	default:
		return "", fmt.Errorf("unknown SEASON_PRECISION %q (want series|seasons)", s)
	}
}

// SeriesPresent reports whether a series counts as in-library under the given
// precision. In `series` mode, presence of the show suffices (present). In
// `seasons` mode, the caller must additionally confirm every requested season;
// this returns the show's presence and whether season-level verification is
// still required, which the provisioner (Phase 6/7) performs via CountSeasons.
//
// Phase 5 wires the policy and the default path; per-season verification against
// the live library is completed when the provisioner consumes it.
func (p SeasonPrecision) SeriesPresent(showPresent bool) (present, needsSeasonCheck bool) {
	if !showPresent {
		return false, false
	}
	if p == PrecisionSeasons {
		return false, true // show exists but seasons not yet verified
	}
	return true, false
}
