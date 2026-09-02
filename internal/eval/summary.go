//go:build eval

package eval

import (
	"fmt"
	"strings"
)

// HumanSummary renders a stable Markdown comparison summary from the same
// Scorecard that is serialized for machines.
func HumanSummary(card Scorecard) string {
	passed := 0
	for _, result := range card.Results {
		if result.Passed() {
			passed++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Planner model certification: %s / %s\n\n", card.Generator.Provider, card.Generator.Model)
	fmt.Fprintf(&b, "Corpus `%s`; %d/%d trials passed; certified: %t.\n\n", card.CorpusVersion, passed, len(card.Results), card.Certified)
	b.WriteString("## Hard gates\n\n")
	if card.Contract == nil || len(card.Contract.HardMetrics) == 0 {
		b.WriteString("No certification hard-gate manifest attached.\n")
	} else {
		fmt.Fprintf(&b, "%s.\n", strings.Join(card.Contract.HardMetrics, ", "))
	}
	b.WriteString("\n## Quality metrics\n\n")
	if card.Contract == nil || len(card.Contract.QualityMetrics) == 0 {
		b.WriteString("No certification quality-metric manifest attached.\n")
	} else {
		fmt.Fprintf(&b, "%s.\n", strings.Join(card.Contract.QualityMetrics, ", "))
	}
	b.WriteString("\n| Case | Trial | Result | Tool calls | Candidates | Theme fit | Judge |\n")
	b.WriteString("| --- | ---: | --- | ---: | ---: | ---: | ---: |\n")
	for _, result := range card.Results {
		status := "PASS"
		if !result.Passed() {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %d | %d | %.2f | %.2f |\n",
			result.Case, result.Trial, status, result.ToolCalls, result.CandidatesSurfaced,
			result.ThemeFit, result.JudgeScore)
	}
	return b.String()
}
