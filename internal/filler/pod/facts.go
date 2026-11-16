package pod

// FactsComplete reports whether a clip carries the facts a break needs (#3838 decision 6): a role,
// an era and an audience. A clip missing one fits no channel and never airs, whatever the
// channel's audience or era. Country is the geography boundary's to judge
// (GeographicallyEligible), which already refuses an unknown country once the installation has one.
func FactsComplete(c Clip) bool {
	return c.Role.ValidConcrete() && c.Era > 0 && c.Audience != ""
}

// filterFacts keeps the clips that carry every fact a break needs.
func filterFacts(in []Clip) []Clip {
	out := make([]Clip, 0, len(in))
	for _, c := range in {
		if FactsComplete(c) {
			out = append(out, c)
		}
	}
	return out
}
