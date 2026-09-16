//go:build eval

package eval

import "github.com/loomarr/loomarr/internal/llm"

// QuerySourceReview records the limits of an independently reviewed source.
// These records are bound by the corpus manifest digest, not fetched in tests.
type QuerySourceReview struct {
	URL         string `json:"url"`
	Observed    string `json:"observed"`
	Role        string `json:"role"`
	Summary     string `json:"summary"`
	Uncertainty string `json:"uncertainty"`
}

const queryExpansionManifest = "testdata/query-expansion-v2.json"

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

func QueryExpansionRunnerConfig(config RunnerConfig) (RunnerConfig, error) {
	return queryDevelopmentRunnerConfig(queryExpansionManifest, "query-expansion-scorer-v2", config)
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
