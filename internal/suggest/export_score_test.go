package suggest

import "github.com/loomarr/loomarr/internal/proposal"

// ScoreForTest exposes the production scorer without supplying date constraints.
func ScoreForTest(request proposal.Intent, lineup, acquisitions []proposal.ProposalItem) proposal.Scores {
	return score(Intent{Intent: request}, lineup, acquisitions, ValidatedDateMeaning{})
}

func ScoreWithDateForTest(request proposal.Intent, lineup, acquisitions []proposal.ProposalItem, meaning ValidatedDateMeaning) proposal.Scores {
	return score(Intent{Intent: request}, lineup, acquisitions, meaning)
}

func ThemeFitForTest(request proposal.Intent, lineup, acquisitions []proposal.ProposalItem) *float64 {
	value, _ := themeFit(Intent{Intent: request}, append(append([]proposal.ProposalItem{}, lineup...), acquisitions...))
	return value
}
