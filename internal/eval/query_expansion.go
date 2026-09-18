//go:build eval

package eval

import (
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/llm"
)

// QuerySourceReview records the limits of an independently reviewed source.
// These records are bound by the corpus manifest digest, not fetched in tests.
type QuerySourceReview struct {
	URL         string `json:"url"`
	Observed    string `json:"observed"`
	Role        string `json:"role"`
	Summary     string `json:"summary"`
	Uncertainty string `json:"uncertainty"`
}

const queryExpansionManifest = "testdata/query-expansion-v8.json"
const queryExpansionDiagnosticMaxInputTokens = 40000

type QueryExpansionDiagnosticSelection struct {
	ID         string `json:"id"`
	Capability string `json:"capability"`
}

var queryExpansionDiagnosticSelection = []QueryExpansionDiagnosticSelection{
	{ID: "exp-tgif-minimum", Capability: "named programming block"},
	{ID: "exp-history-minimum", Capability: "network editorial era"},
	{ID: "exp-movie-epoch-1940s", Capability: "movie release era"},
	{ID: "exp-movie-mood-comforting", Capability: "movie mood"},
	{ID: "exp-movie-audience-pg-or-lower", Capability: "audience ceiling"},
	{ID: "exp-movie-exclude-modern-two", Capability: "title exclusions"},
	{ID: "exp-movie-thin-library-2010s", Capability: "thin library result"},
	{ID: "exp-movie-audience-empty-excluded-toy-story", Capability: "valid empty result"},
	{ID: "exp-movie-refine-add", Capability: "add refinement"},
	{ID: "exp-movie-refine-remove", Capability: "remove refinement"},
}

// LoadEmbeddedQueryExpansionCorpus loads exposed development evidence only.
func LoadEmbeddedQueryExpansionCorpus() (QueryPilotCorpus, error) {
	return loadQueryDevelopmentCorpus(queryExpansionManifest)
}

// QueryExpansionCases projects reviewed requests onto the existing Runner seam.
func QueryExpansionCases() ([]Case, error) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		return nil, err
	}
	return queryDevelopmentCases(corpus), nil
}

// QueryExpansionDiagnosticCases returns the reviewed, ordered behavior sample
// used by the explicit hosted-model diagnostic. It is development evidence,
// never release certification.
func QueryExpansionDiagnosticCases() ([]Case, error) {
	cases, err := QueryExpansionCases()
	if err != nil {
		return nil, err
	}
	byName := make(map[string]Case, len(cases))
	for _, candidate := range cases {
		if _, duplicate := byName[candidate.Name]; duplicate {
			return nil, fmt.Errorf("query expansion diagnostic case %q is duplicated", candidate.Name)
		}
		byName[candidate.Name] = candidate
	}
	selected := make([]Case, 0, len(queryExpansionDiagnosticSelection))
	for _, selection := range queryExpansionDiagnosticSelection {
		candidate, ok := byName[selection.ID]
		if !ok {
			return nil, fmt.Errorf("query expansion diagnostic case %q is missing", selection.ID)
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

func QueryExpansionDiagnosticSelections() []QueryExpansionDiagnosticSelection {
	return append([]QueryExpansionDiagnosticSelection(nil), queryExpansionDiagnosticSelection...)
}

// QueryExpansionDiagnosticSummary keeps exposed development results readable by
// behavior instead of presenting one undifferentiated pass count.
func QueryExpansionDiagnosticSummary(card Scorecard) string {
	capabilities := make(map[string]string, len(queryExpansionDiagnosticSelection))
	for _, selection := range queryExpansionDiagnosticSelection {
		capabilities[selection.ID] = selection.Capability
	}
	var summary strings.Builder
	summary.WriteString("# Query expansion diagnostic\n\n")
	fmt.Fprintf(&summary, "Model `%s` through `%s`; corpus `%s`.\n\n", card.Generator.Model, card.Generator.Provider, card.CorpusVersion)
	fmt.Fprintf(&summary, "Development evidence only; certified: %t.\n\n", card.Certified)
	fmt.Fprintf(&summary, "Hard ceilings: %d calls, %d tokens, USD %s. Per-call route, token, and charge evidence remains in the JSON scorecard.\n\n",
		card.CallBudget.Total, card.CallBudget.Resource.MaxTokensPerSuite, card.CallBudget.Resource.MaxSpendPerSuite)
	summary.WriteString("| Capability | Case | Result | Failure stage | Detail |\n")
	summary.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, result := range card.Results {
		status := "PASS"
		if !result.Passed() {
			status = "FAIL"
		}
		capability := capabilities[result.Case]
		if capability == "" {
			capability = "unclassified"
		}
		detail := strings.Join(result.Failures, "; ")
		fmt.Fprintf(&summary, "| %s | %s | %s | %s | %s |\n",
			markdownTableText(capability), markdownTableText(result.Case), status,
			markdownTableText(string(result.FailureStage)), markdownTableText(detail))
	}
	return summary.String()
}

func markdownTableText(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func QueryExpansionRunnerConfig(config RunnerConfig) (RunnerConfig, error) {
	return queryDevelopmentRunnerConfig(queryExpansionManifest, "query-expansion-scorer-v8", config)
}

// NewEmbeddedQueryExpansionGenerator varies fixture facts, never grounding or
// resource ceilings. The caller supplies the only model provider dependency.
func NewEmbeddedQueryExpansionGenerator(provider llm.Provider) (Generator, Observer, error) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		return nil, nil, err
	}
	return newQueryDevelopmentGenerator(corpus, provider)
}
