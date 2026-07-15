package suggest

// export_score_test.go bridges the unexported deterministic scoring functions to
// the external suggest_test package so the property tests in
// score_property_test.go can exercise them directly (not only through the full
// Suggest pipeline). Test-only: this file has the _test.go suffix and ships no
// production symbols.

// ScoreForTest exposes score() (§8 deterministic post-scoring).
func ScoreForTest(intent Intent, lineup, acquisitions []ProposalItem) Scores {
	return score(intent, lineup, acquisitions)
}

// ThemeFitForTest exposes themeFit().
func ThemeFitForTest(intent Intent, lineup, acquisitions []ProposalItem) float64 {
	return themeFit(intent, lineup, acquisitions)
}

// EraBalanceForTest exposes eraBalance().
func EraBalanceForTest(intent Intent, lineup, acquisitions []ProposalItem) float64 {
	return eraBalance(intent, lineup, acquisitions)
}

// CompositeForTest exposes composite().
func CompositeForTest(s Scores) float64 {
	return composite(s)
}
