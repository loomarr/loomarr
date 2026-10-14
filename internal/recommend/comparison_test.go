package recommend_test

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestCompareKeepsSharedPlannerModelWhenAlternativeDoesNotClearMargin(t *testing.T) {
	shared := passingCard("shared-planner", "ollama", "qwen:9b", 0.90)
	alternative := passingCard("recommendation-only", "ollama", "gemma:12b", 0.91)
	comparison, err := recommend.Compare([]recommend.Scorecard{shared, alternative}, "shared-planner")
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Decision != recommend.DecisionSharedSufficient || comparison.SelectedProfile != "shared-planner" {
		t.Fatalf("comparison = %+v", comparison)
	}
	if summary := recommend.HumanComparison(comparison); !strings.Contains(summary, "shared model is sufficient") {
		t.Fatalf("summary = %s", summary)
	}
}

func TestCompareJustifiesDistinctRouteOnlyAfterCertifiedMargin(t *testing.T) {
	shared := passingCard("shared-planner", "ollama", "qwen:9b", 0.90)
	alternative := passingCard("recommendation-only", "ollama", "gemma:12b", 0.93)
	comparison, err := recommend.Compare([]recommend.Scorecard{shared, alternative}, "shared-planner")
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Decision != recommend.DecisionDistinctJustified || comparison.SelectedProfile != "recommendation-only" {
		t.Fatalf("comparison = %+v", comparison)
	}
}

func TestCompareRejectsScorecardsFromDifferentFrozenContracts(t *testing.T) {
	shared := passingCard("shared-planner", "ollama", "qwen:9b", 0.90)
	alternative := passingCard("other", "openrouter", "vendor/model", 0.95)
	alternative.CorpusSHA256 = "different"
	_, err := recommend.Compare([]recommend.Scorecard{shared, alternative}, "shared-planner")
	if err == nil || !strings.Contains(err.Error(), "frozen contract") {
		t.Fatalf("comparison error = %v", err)
	}
}

func passingCard(profile, provider, model string, score float64) recommend.Scorecard {
	thresholds := recommend.Thresholds{
		MinRelevance: 0.8, MinNovelty: 0.75, MinDiversity: 0.7, MinCatalogFeasibility: 0.9,
		MinPolicySafety: 1, MinSchemaValidity: 0.98, MinAbstention: 0.8,
	}
	return recommend.Scorecard{
		SchemaVersion: 1, CorpusVersion: "channel-recommendation-v1", CorpusSHA256: "fixture",
		PromptVersion: "prompt-v1", OutputSchema: "schema-v1", ScorerVersion: "scorer-v1",
		SelectionMetric: "mean_quality", SelectionMargin: 0.02, Thresholds: thresholds,
		Profile: profile, Provider: provider, Model: model, Certified: true,
		Budget: recommend.RunConfig{ExpectedCases: 1}, Results: []recommend.CaseRun{{CaseResult: recommend.CaseResult{Passed: true}}},
		Quality: recommend.AggregateQuality{Quality: recommend.Quality{
			Relevance: score, Novelty: score, Diversity: score, CatalogFeasibility: score,
			PolicySafety: 1, Abstention: score,
		}, SchemaValidity: 1},
		HardFailureCounts: map[string]int{}, Resources: recommend.ResourceUsage{AccountingComplete: true},
	}
}
