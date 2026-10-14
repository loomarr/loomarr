package suggest

// ScoreForTest exposes the production scorer without supplying date constraints.
func ScoreForTest(intent Intent, lineup, acquisitions []ProposalItem) Scores {
	return score(intent, lineup, acquisitions, ValidatedDateMeaning{})
}

func ScoreWithDateForTest(intent Intent, lineup, acquisitions []ProposalItem, meaning ValidatedDateMeaning) Scores {
	return score(intent, lineup, acquisitions, meaning)
}

func ThemeFitForTest(intent Intent, lineup, acquisitions []ProposalItem) *float64 {
	value, _ := themeFit(intent, append(append([]ProposalItem{}, lineup...), acquisitions...))
	return value
}
