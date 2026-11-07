package recommend

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DecisionSharedSufficient  = "shared_model_sufficient"
	DecisionDistinctJustified = "distinct_recommendation_route_justified"
	DecisionNoCandidate       = "no_candidate_certified"
	DecisionMissingShared     = "inconclusive_missing_shared_model"
)

type CandidateComparison struct {
	Profile      string  `json:"profile"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Certified    bool    `json:"certified"`
	MeanQuality  float64 `json:"meanQuality"`
	Calls        int     `json:"calls"`
	Tokens       int     `json:"tokens"`
	SpendNanoUSD int64   `json:"spendNanoUsd"`
}

type Comparison struct {
	SchemaVersion   int                   `json:"schemaVersion"`
	CorpusVersion   string                `json:"corpusVersion"`
	CorpusSHA256    string                `json:"corpusSha256"`
	PromptVersion   string                `json:"promptVersion"`
	OutputSchema    string                `json:"outputSchema"`
	ScorerVersion   string                `json:"scorerVersion"`
	SelectionMetric string                `json:"selectionMetric"`
	SelectionMargin float64               `json:"selectionMargin"`
	SharedProfile   string                `json:"sharedProfile"`
	Candidates      []CandidateComparison `json:"candidates"`
	Decision        string                `json:"decision"`
	SelectedProfile string                `json:"selectedProfile,omitempty"`
	Reason          string                `json:"reason"`
}

func Compare(cards []Scorecard, sharedProfile string) (Comparison, error) {
	if len(cards) < 2 {
		return Comparison{}, fmt.Errorf("recommendation comparison requires at least two scorecards")
	}
	sharedProfile = strings.TrimSpace(sharedProfile)
	if sharedProfile == "" {
		return Comparison{}, fmt.Errorf("recommendation comparison requires the shared planner profile")
	}
	first := cards[0]
	if first.SchemaVersion != 1 || first.CorpusVersion == "" || first.CorpusSHA256 == "" ||
		first.PromptVersion == "" || first.OutputSchema == "" || first.ScorerVersion == "" {
		return Comparison{}, fmt.Errorf("recommendation scorecard lacks its frozen contract identity")
	}
	comparison := Comparison{
		SchemaVersion: 1, CorpusVersion: first.CorpusVersion, CorpusSHA256: first.CorpusSHA256,
		PromptVersion: first.PromptVersion, OutputSchema: first.OutputSchema, ScorerVersion: first.ScorerVersion,
		SelectionMetric: first.SelectionMetric, SelectionMargin: first.SelectionMargin, SharedProfile: sharedProfile,
	}
	seenProfiles := make(map[string]bool, len(cards))
	sharedIndex := -1
	for _, card := range cards {
		if !sameComparisonContract(first, card) {
			return Comparison{}, fmt.Errorf("recommendation scorecards do not share one frozen contract")
		}
		if card.Profile == "" || seenProfiles[card.Profile] {
			return Comparison{}, fmt.Errorf("recommendation scorecard profiles must be non-empty and unique")
		}
		seenProfiles[card.Profile] = true
		candidate := CandidateComparison{
			Profile: card.Profile, Provider: card.Provider, Model: card.Model,
			Certified: eligibleScorecard(card), MeanQuality: meanQuality(card.Quality),
			Calls: card.Resources.Calls, Tokens: card.Resources.Tokens, SpendNanoUSD: card.Resources.SpendNanoUSD,
		}
		comparison.Candidates = append(comparison.Candidates, candidate)
		if card.Profile == sharedProfile {
			sharedIndex = len(comparison.Candidates) - 1
		}
	}
	if sharedIndex < 0 {
		comparison.Decision = DecisionMissingShared
		comparison.Reason = "The declared shared planner profile has no comparable scorecard."
		return comparison, nil
	}
	shared := comparison.Candidates[sharedIndex]
	bestAlternative := CandidateComparison{}
	for index, candidate := range comparison.Candidates {
		if index == sharedIndex || !candidate.Certified {
			continue
		}
		if bestAlternative.Profile == "" || candidate.MeanQuality > bestAlternative.MeanQuality {
			bestAlternative = candidate
		}
	}
	switch {
	case shared.Certified && (bestAlternative.Profile == "" || bestAlternative.MeanQuality < shared.MeanQuality+comparison.SelectionMargin):
		comparison.Decision = DecisionSharedSufficient
		comparison.SelectedProfile = shared.Profile
		comparison.Reason = "The shared model is sufficient: no certified alternative cleared the pre-registered selection margin."
	case bestAlternative.Profile != "" && (!shared.Certified || bestAlternative.MeanQuality >= shared.MeanQuality+comparison.SelectionMargin):
		comparison.Decision = DecisionDistinctJustified
		comparison.SelectedProfile = bestAlternative.Profile
		comparison.Reason = "A distinct recommendation route is justified by certification and the pre-registered selection margin."
	default:
		comparison.Decision = DecisionNoCandidate
		comparison.Reason = "No candidate cleared the complete recommendation certification contract."
	}
	sort.SliceStable(comparison.Candidates, func(i, j int) bool {
		if comparison.Candidates[i].MeanQuality == comparison.Candidates[j].MeanQuality {
			return comparison.Candidates[i].Profile < comparison.Candidates[j].Profile
		}
		return comparison.Candidates[i].MeanQuality > comparison.Candidates[j].MeanQuality
	})
	return comparison, nil
}

func HumanComparison(comparison Comparison) string {
	var summary strings.Builder
	summary.WriteString("# Channel recommendation model comparison\n\n")
	fmt.Fprintf(&summary, "Decision: **%s**. %s\n\n", comparison.Decision, comparison.Reason)
	fmt.Fprintf(&summary, "Frozen contract `%s` (`%s`), selection `%s` with margin %.3f.\n\n",
		comparison.CorpusVersion, comparison.CorpusSHA256, comparison.SelectionMetric, comparison.SelectionMargin)
	summary.WriteString("| Profile | Provider / model | Certified | Mean quality | Calls | Tokens | Spend |\n")
	summary.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, candidate := range comparison.Candidates {
		fmt.Fprintf(&summary, "| %s | %s / %s | %t | %.3f | %d | %d | $%s |\n",
			candidate.Profile, candidate.Provider, candidate.Model, candidate.Certified, candidate.MeanQuality,
			candidate.Calls, candidate.Tokens, formatNanoUSD(candidate.SpendNanoUSD))
	}
	return summary.String()
}

func sameComparisonContract(left, right Scorecard) bool {
	return left.SchemaVersion == right.SchemaVersion && left.CorpusVersion == right.CorpusVersion &&
		left.CorpusSHA256 == right.CorpusSHA256 && left.PromptVersion == right.PromptVersion &&
		left.OutputSchema == right.OutputSchema && left.ScorerVersion == right.ScorerVersion &&
		left.SelectionMetric == "mean_quality" && right.SelectionMetric == left.SelectionMetric &&
		left.SelectionMargin > 0 && right.SelectionMargin == left.SelectionMargin && left.Thresholds == right.Thresholds
}

func eligibleScorecard(card Scorecard) bool {
	return card.Certified && card.StopReason == "" && len(card.Results) == card.Budget.ExpectedCases &&
		card.Resources.AccountingComplete && len(card.HardFailureCounts) == 0 && validQuality(card.Quality) &&
		meetsThresholds(card.Quality, card.Thresholds)
}

func meanQuality(quality AggregateQuality) float64 {
	return (quality.Relevance + quality.Novelty + quality.Diversity + quality.CatalogFeasibility +
		quality.PolicySafety + quality.SchemaValidity + quality.Abstention) / 7
}

func validQuality(quality AggregateQuality) bool {
	values := []float64{quality.Relevance, quality.Novelty, quality.Diversity, quality.CatalogFeasibility,
		quality.PolicySafety, quality.SchemaValidity, quality.Abstention}
	for _, value := range values {
		if value < 0 || value > 1 {
			return false
		}
	}
	return true
}
