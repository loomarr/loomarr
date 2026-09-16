//go:build eval

package eval

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

//go:embed testdata/query-pilot-v1.json testdata/query-pilot-catalog-v1.json testdata/query-pilot-sources-v1.json testdata/query-expansion-v1.json testdata/query-expansion-catalog-v1.json testdata/query-expansion-sources-v1.json testdata/query-expansion-v2.json testdata/query-expansion-catalog-v2.json testdata/query-expansion-sources-v2.json testdata/query-expansion-v3.json testdata/query-expansion-catalog-v3.json
var queryPilotFiles embed.FS

// QueryPilotCorpus is exposed development evidence, never a release holdout.
type QueryPilotCorpus struct {
	SchemaVersion       int                  `json:"schemaVersion"`
	Version             string               `json:"version"`
	Split               string               `json:"split"`
	PromptVersion       string               `json:"promptVersion"`
	ToolSchemaVersion   string               `json:"toolSchemaVersion"`
	SourceVersion       string               `json:"sourceVersion"`
	CatalogFixture      CertificationFixture `json:"catalogFixture"`
	SourcesFixture      CertificationFixture `json:"sourcesFixture"`
	ExtraCatalogFixture CertificationFixture `json:"extraCatalogFixture"`
	SourceReviews       []QuerySourceReview  `json:"sourceReviews,omitempty"`
	Cases               []QueryPilotCase     `json:"cases"`
}

type QueryPilotCase struct {
	ID              string              `json:"id"`
	Group           string              `json:"group"`
	Category        string              `json:"category"`
	Type            string              `json:"type"`
	FixtureCase     string              `json:"fixtureCase"`
	Description     string              `json:"description"`
	MustInclude     []string            `json:"mustInclude,omitempty"`
	MustExclude     []string            `json:"mustExclude,omitempty"`
	MinGrounded     int                 `json:"minGrounded"`
	MinLineup       int                 `json:"minLineup,omitempty"`
	MinAcquisitions int                 `json:"minAcquisitions,omitempty"`
	AcceptableKeys  []provision.Key     `json:"acceptableKeys"`
	RequireKeys     []provision.Key     `json:"requireKeys,omitempty"`
	ForbidKeys      []provision.Key     `json:"forbidKeys,omitempty"`
	NoDates         bool                `json:"noDates,omitempty"`
	Dates           *schedule.DateScope `json:"dates,omitempty"`
}

// LoadEmbeddedQueryPilotCorpus validates identity and facts before any provider
// can be constructed by a caller. The manifest is authored, not a paraphrase loop.
func LoadEmbeddedQueryPilotCorpus() (QueryPilotCorpus, error) {
	return loadQueryDevelopmentCorpus("testdata/query-pilot-v1.json")
}

func loadQueryDevelopmentCorpus(path string) (QueryPilotCorpus, error) {
	blob, err := queryPilotFiles.ReadFile(path)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	var corpus QueryPilotCorpus
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot manifest has trailing data")
	}
	if corpus.SchemaVersion != 1 || corpus.Version == "" || corpus.Split != "development" || len(corpus.Cases) == 0 {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot development identity is incomplete")
	}
	if corpus.PromptVersion != suggest.PlannerPromptVersion || corpus.ToolSchemaVersion != suggest.PlannerToolSchemaVersion || corpus.SourceVersion != suggest.PlannerSourceVersion {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot differs from the production planner contract")
	}
	for _, binding := range []CertificationFixture{corpus.CatalogFixture, corpus.SourcesFixture, corpus.ExtraCatalogFixture} {
		facts, err := readQueryPilotFacts(binding.Path)
		if err != nil {
			return QueryPilotCorpus{}, err
		}
		digest := sha256.Sum256(facts)
		if hex.EncodeToString(digest[:]) != binding.SHA256 {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot fixture digest mismatch: %s", binding.Path)
		}
	}
	factsBlob, err := releaseGateFiles.ReadFile(corpus.CatalogFixture.Path)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	var facts certificationCatalogFixture
	if err := json.Unmarshal(factsBlob, &facts); err != nil {
		return QueryPilotCorpus{}, err
	}
	extraBlob, err := readQueryPilotFacts(corpus.ExtraCatalogFixture.Path)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	var extra certificationCatalogFixture
	if err := json.Unmarshal(extraBlob, &extra); err != nil {
		return QueryPilotCorpus{}, err
	}
	facts.Cases = append(facts.Cases, extra.Cases...)
	keysByFixture := make(map[string]map[provision.Key]bool)
	for _, fixture := range facts.Cases {
		keys, err := fixtureCandidateKeys(fixture)
		if err != nil {
			return QueryPilotCorpus{}, err
		}
		keysByFixture[fixture.ID] = keys
	}
	seen := make(map[string]bool)
	descriptions := make(map[string]bool)
	for _, c := range corpus.Cases {
		if c.ID == "" || c.Group == "" || c.Category == "" || c.Description == "" || seen[c.ID] || descriptions[c.Description] {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot has missing or duplicate case identity: %q", c.ID)
		}
		seen[c.ID], descriptions[c.Description] = true, true
		if c.Type != "minimum" && c.Type != "invariance" && c.Type != "directional" {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has unsupported test type", c.ID)
		}
		if c.MinGrounded < 1 || c.MinGrounded > len(c.AcceptableKeys) || c.MinLineup < 0 || c.MinAcquisitions < 0 || c.MinLineup+c.MinAcquisitions > len(c.AcceptableKeys) {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has impossible breadth or ownership expectations", c.ID)
		}
		if c.NoDates == (c.Dates != nil) || (c.Dates != nil && !normalizedDateScope(c.Dates)) {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q must declare none or canonical date axes", c.ID)
		}
		known := keysByFixture[c.FixtureCase]
		if known == nil {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has no source fixture", c.ID)
		}
		allowed := make(map[provision.Key]bool)
		for _, key := range c.AcceptableKeys {
			if !known[key] || allowed[key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has unknown or duplicate acceptable key %q", c.ID, key)
			}
			allowed[key] = true
		}
		for _, key := range c.RequireKeys {
			if !allowed[key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q requires an unacceptable key", c.ID)
			}
		}
		for _, key := range c.ForbidKeys {
			if !known[key] || allowed[key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has absent or conflicting negative evidence", c.ID)
			}
		}
	}
	return corpus, nil
}

func readQueryPilotFacts(path string) ([]byte, error) {
	if blob, err := queryPilotFiles.ReadFile(path); err == nil {
		return blob, nil
	}
	return releaseGateFiles.ReadFile(path)
}

// NewEmbeddedQueryPilotGenerator reuses the production Suggester, observed
// provider and shared Catalog adapters. Only the caller's provider can vary.
func NewEmbeddedQueryPilotGenerator(provider llm.Provider) (Generator, Observer, error) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		return nil, nil, err
	}
	return newQueryDevelopmentGenerator(corpus, provider)
}

func newQueryDevelopmentGenerator(corpus QueryPilotCorpus, provider llm.Provider) (Generator, Observer, error) {
	var decoded certificationCatalogFixture
	for _, binding := range []CertificationFixture{corpus.CatalogFixture, corpus.ExtraCatalogFixture} {
		blob, err := readQueryPilotFacts(binding.Path)
		if err != nil {
			return nil, nil, err
		}
		var facts certificationCatalogFixture
		if err := json.Unmarshal(blob, &facts); err != nil {
			return nil, nil, err
		}
		decoded.Cases = append(decoded.Cases, facts.Cases...)
	}
	decoded.SchemaVersion, decoded.FixtureID = 1, corpus.Version
	fixture := newEmbeddedCatalogFixture(decoded)
	observed := &observedProvider{inner: provider}
	cat := catalog.New(embeddedLibraryCatalog{fixture}, embeddedTMDBCatalog{fixture}).WithPresence(fixture)
	sourceBlob, err := readQueryPilotFacts(corpus.SourcesFixture.Path)
	if err != nil {
		return nil, nil, err
	}
	references := &testkit.ReferenceResolver{ByURL: make(map[string]reference.Evidence)}
	if err := json.Unmarshal(sourceBlob, &references.ByLabel); err != nil {
		return nil, nil, err
	}
	// The immutable pilot predates stable acronym-subject extraction and stores
	// its synthetic source under the qualified label. Keep those bytes runnable
	// while production discovery uses the canonical acronym.
	if legacy, found := references.ByLabel["MCU Phase One"]; found {
		if _, current := references.ByLabel["MCU"]; !current {
			references.ByLabel["MCU"] = legacy
		}
	}
	bindings := make(map[string]string, len(corpus.Cases))
	for _, c := range corpus.Cases {
		bindings[c.Description] = c.FixtureCase
	}
	inner := suggest.New(observed, cat, fixture, 10).WithRatings(fixture).WithReferences(references)
	generator := &releaseGateGenerator{inner: inner, fixture: fixture, caseByIntent: bindings}
	return generator, &releaseGateObserver{provider: observed, fixture: fixture, generator: generator}, nil
}

func QueryPilotCases() ([]Case, error) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		return nil, err
	}
	return queryDevelopmentCases(corpus), nil
}

func queryDevelopmentCases(corpus QueryPilotCorpus) []Case {
	cases := make([]Case, 0, len(corpus.Cases))
	for _, authored := range corpus.Cases {
		dates := cloneDateScope(authored.Dates)
		if authored.NoDates {
			dates = &schedule.DateScope{}
		}
		cases = append(cases, Case{
			Name: authored.ID, Intent: Intent{Description: authored.Description, MustInclude: authored.MustInclude, MustExclude: authored.MustExclude},
			NoFabrication: true, RequireUniqueKeys: true, OnlyAcceptableKeys: true, ExpectGroundedCompletion: true,
			MinGrounded: authored.MinGrounded, MinLineup: authored.MinLineup, MinAcquisitions: authored.MinAcquisitions,
			AcceptableKeys: authored.AcceptableKeys, MinAcceptableKeys: authored.MinGrounded,
			RequireKeys: authored.RequireKeys, ForbidKeys: authored.ForbidKeys, ExpectedDateScope: dates,
		})
	}
	return withProductionStructuralBounds(cases)
}

// QueryPilotRunnerConfig makes development status explicit and retains strict
// policy/schema/completion assessment without modifying any certification bar.
func QueryPilotRunnerConfig(config RunnerConfig) (RunnerConfig, error) {
	return queryDevelopmentRunnerConfig("testdata/query-pilot-v1.json", "query-pilot-scorer-v1", config)
}

func queryDevelopmentRunnerConfig(path, scorer string, config RunnerConfig) (RunnerConfig, error) {
	corpus, err := loadQueryDevelopmentCorpus(path)
	if err != nil {
		return RunnerConfig{}, err
	}
	config.DevelopmentCorpus = true
	manifest, err := queryPilotFiles.ReadFile(path)
	if err != nil {
		return RunnerConfig{}, err
	}
	manifestDigest := sha256.Sum256(manifest)
	config.Contract = &CertificationContract{
		CorpusManifestSHA256:             hex.EncodeToString(manifestDigest[:]),
		SourcesFixtureSHA256:             corpus.SourcesFixture.SHA256,
		SupplementalCatalogFixtureSHA256: corpus.ExtraCatalogFixture.SHA256,
		SourceVersion:                    corpus.SourceVersion,
		CorpusVersion:                    corpus.Version, CatalogFixtureSHA256: corpus.CatalogFixture.SHA256,
		PromptVersion: corpus.PromptVersion, ToolSchemaVersion: corpus.ToolSchemaVersion, ScorerVersion: scorer,
		HardMetrics:    []string{"grounding", "required_anchors", "forbidden_matches", "distinct_breadth", "ownership"},
		QualityMetrics: []string{"policy_accuracy"},
		Thresholds:     CertificationThresholds{MinGroundedCompletionRate: 1, MinSchemaValidityRate: 1, MinPolicyAccuracyRate: 1, MaxP95ToolCalls: suggest.ProductionBounds().MaxToolCalls},
	}
	return config, nil
}
