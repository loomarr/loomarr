package suggest

import "github.com/loomarr/loomarr/internal/proposals"

// ScoreForTest exposes the production scorer without supplying date constraints.
func ScoreForTest(request proposals.Intent, lineup, acquisitions []proposals.ProposalItem) proposals.Scores {
	return score(Intent{Intent: request}, lineup, acquisitions, ValidatedDateMeaning{})
}

func ScoreWithDateForTest(request proposals.Intent, lineup, acquisitions []proposals.ProposalItem, meaning ValidatedDateMeaning) proposals.Scores {
	return score(Intent{Intent: request}, lineup, acquisitions, meaning)
}

func ThemeFitForTest(request proposals.Intent, lineup, acquisitions []proposals.ProposalItem) *float64 {
	value, _ := themeFit(Intent{Intent: request}, append(append([]proposals.ProposalItem{}, lineup...), acquisitions...))
	return value
}
