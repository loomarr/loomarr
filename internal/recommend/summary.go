package recommend

import (
	"fmt"
	"strings"
)

func HumanSummary(card Scorecard) string {
	status := "NOT CERTIFIED"
	if card.Certified {
		status = "CERTIFIED"
	}
	var summary strings.Builder
	fmt.Fprintf(&summary, "# Channel recommendation certification: %s\n\n", status)
	fmt.Fprintf(&summary, "Corpus `%s` (`%s`), provider `%s`, model `%s`, profile `%s`.\n\n",
		card.CorpusVersion, card.CorpusSHA256, card.Provider, card.Model, card.Profile)
	fmt.Fprintf(&summary, "Resources: %d calls, %d tokens, $%s.\n\n",
		card.Resources.Calls, card.Resources.Tokens, formatNanoUSD(card.Resources.SpendNanoUSD))
	if !card.Resources.AccountingComplete {
		summary.WriteString("Resource accounting is incomplete; the displayed total is not a spend claim.\n\n")
	}
	if card.StopReason != "" {
		fmt.Fprintf(&summary, "Stopped: `%s`.\n\n", card.StopReason)
	}
	summary.WriteString("| Metric | Result | Floor |\n| --- | ---: | ---: |\n")
	rows := []struct {
		name  string
		value float64
		floor float64
	}{
		{"Relevance", card.Quality.Relevance, card.Thresholds.MinRelevance},
		{"Novelty", card.Quality.Novelty, card.Thresholds.MinNovelty},
		{"Diversity", card.Quality.Diversity, card.Thresholds.MinDiversity},
		{"Catalog feasibility", card.Quality.CatalogFeasibility, card.Thresholds.MinCatalogFeasibility},
		{"Policy safety", card.Quality.PolicySafety, card.Thresholds.MinPolicySafety},
		{"Schema validity", card.Quality.SchemaValidity, card.Thresholds.MinSchemaValidity},
		{"Abstention", card.Quality.Abstention, card.Thresholds.MinAbstention},
	}
	for _, row := range rows {
		fmt.Fprintf(&summary, "| %s | %.3f | %.3f |\n", row.name, row.value, row.floor)
	}
	fmt.Fprintf(&summary, "\nCases: %d/%d passed; hard failures: %d.\n", passedCases(card.Results), card.Budget.ExpectedCases, totalHardFailures(card.HardFailureCounts))
	return summary.String()
}

func formatNanoUSD(nano int64) string {
	return fmt.Sprintf("%d.%09d", nano/1_000_000_000, nano%1_000_000_000)
}

func passedCases(results []CaseRun) int {
	passed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		}
	}
	return passed
}

func totalHardFailures(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}
