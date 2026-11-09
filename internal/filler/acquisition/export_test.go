package acquisition

// PlanAcquisition is PlanAcquisitionFor with no coverage gaps. It is a test entry point into
// production code that no binary reaches, so it lives here rather than in the package, where
// deadcode would report it.
func PlanAcquisition(intent AcquisitionIntent, candidates []Candidate, existing map[string]ExistingRemoteState) (Plan, error) {
	return PlanAcquisitionFor(intent, candidates, existing, CoverageGaps{})
}
