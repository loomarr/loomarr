package fillerenrichment

// DetailState is the deliberately small Library projection. Provider names, budgets, retries, and
// pass identities stay behind diagnostics; the ordinary clip panel only needs to know whether
// background work is still moving or whether the confirmed facts are intentionally sparse.
type DetailState string

const (
	DetailAdding   DetailState = "adding_details"
	DetailLimited  DetailState = "details_limited"
	DetailComplete DetailState = "complete"
)

type DetailFact struct {
	Axis     Axis
	Evidence EvidenceKind
}

type DetailProjection struct {
	State DetailState
	Facts []DetailFact
}

// ProjectDetails turns the full axis/evidence model into the quiet, server-owned answer rendered
// by Library. "Complete" means the facts that panel actually presents are known: type, year,
// audience, advertiser, location, and at least one controlled topic. Sparse scheduling cues are not
// treated as missing merely because a clip is not seasonal or does not carry an audience cue.
func ProjectDetails(states []State) DetailProjection {
	if len(states) == 0 {
		return DetailProjection{State: DetailAdding}
	}
	byAxis := make(map[Axis]State, len(states))
	for _, state := range states {
		byAxis[state.Axis] = state
		if state.Status == StatusMissing || state.Status == StatusStale {
			return DetailProjection{State: DetailAdding}
		}
	}
	projection := DetailProjection{State: DetailLimited}
	for _, axis := range Axes() {
		state, found := byAxis[axis]
		if !found || state.Status != StatusComplete || state.Value.empty() {
			continue
		}
		projection.Facts = append(projection.Facts, DetailFact{Axis: axis, Evidence: state.Evidence.Kind})
	}
	known := func(axis Axis) bool {
		state, found := byAxis[axis]
		return found && state.Status == StatusComplete && !state.Value.empty()
	}
	hasTopic := known(AxisProduct) || known(AxisFormat) || known(AxisSeasonal) ||
		known(AxisAudienceCue) || known(AxisPresentation)
	if known(AxisKind) && known(AxisEra) && known(AxisAudience) && known(AxisBrand) &&
		known(AxisGeography) && hasTopic {
		projection.State = DetailComplete
	}
	return projection
}
