//go:build eval

package eval

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

const releaseGateManifestPath = "testdata/planner-release-gate-v10.json"

//go:embed testdata/planner-release-gate-v1.json testdata/planner-release-gate-v2.json testdata/planner-release-gate-v3.json testdata/planner-release-gate-v4.json testdata/planner-release-gate-v5.json testdata/planner-release-catalog-v1.json testdata/planner-release-sources-v1.json testdata/planner-release-gate-v6.json testdata/planner-release-gate-v7.json testdata/planner-release-gate-v8.json testdata/planner-release-gate-v9.json testdata/planner-release-gate-v10.json
var releaseGateFiles embed.FS

var releaseGateModelCanaryCaseIDs = []string{
	"tgif-terse",
	"tgif-conversational",
	"named-collection",
	"known-franchise",
	"movie-person",
	"tv-network-genre",
	"mood-theme",
	"public-reference",
}

type ReleaseGateCorpus struct {
	SchemaVersion     int                     `json:"schemaVersion"`
	Version           string                  `json:"version"`
	Split             string                  `json:"split"`
	PromptVersion     string                  `json:"promptVersion"`
	ToolSchemaVersion string                  `json:"toolSchemaVersion"`
	SourceVersion     string                  `json:"sourceVersion"`
	ScorerVersion     string                  `json:"scorerVersion"`
	HardMetrics       []string                `json:"hardMetrics"`
	QualityMetrics    []string                `json:"qualityMetrics"`
	Thresholds        CertificationThresholds `json:"thresholds"`
	SourcesFixture    CertificationFixture    `json:"sourcesFixture"`
	Fixture           CertificationFixture    `json:"fixture"`
	Cases             []ReleaseGateCorpusCase `json:"cases"`
}

type ReleaseGateCorpusCase struct {
	ID                string   `json:"id"`
	Lane              string   `json:"lane"`
	FixtureCase       string   `json:"fixtureCase"`
	Description       string   `json:"description"`
	AcceptableKeys    []string `json:"acceptableKeys,omitempty"`
	MinAcceptableKeys int      `json:"minAcceptableKeys,omitempty"`
	ForbidKeys        []string `json:"forbidKeys,omitempty"`
	RequireKeys       []string `json:"requireKeys,omitempty"`
	MinGrounded       int      `json:"minGrounded,omitempty"`
	ExpectEraFrom     int      `json:"expectEraFrom,omitempty"`
	ExpectEraTo       int      `json:"expectEraTo,omitempty"`
	ReferenceURL      string   `json:"referenceUrl,omitempty"`
	ReferenceTitles   []string `json:"referenceTitles,omitempty"`
}

func LoadEmbeddedReleaseGateCorpus() (ReleaseGateCorpus, error) {
	blob, err := releaseGateFiles.ReadFile(releaseGateManifestPath)
	if err != nil {
		return ReleaseGateCorpus{}, fmt.Errorf("read release-gate manifest: %w", err)
	}
	var corpus ReleaseGateCorpus
	if err := json.Unmarshal(blob, &corpus); err != nil {
		return ReleaseGateCorpus{}, fmt.Errorf("decode release-gate manifest: %w", err)
	}
	if corpus.SchemaVersion != 1 || corpus.Version == "" || corpus.Split != "release_holdout" || corpus.ScorerVersion == "" {
		return ReleaseGateCorpus{}, fmt.Errorf("release-gate identity is incomplete")
	}
	if corpus.SourceVersion != suggest.PlannerSourceVersion || corpus.PromptVersion != suggest.PlannerPromptVersion || corpus.ToolSchemaVersion != suggest.PlannerToolSchemaVersion {
		return ReleaseGateCorpus{}, fmt.Errorf("release-gate prompt/tool identity differs from production Suggester")
	}
	fixtureBlob, err := releaseGateFiles.ReadFile(corpus.Fixture.Path)
	if err != nil {
		return ReleaseGateCorpus{}, fmt.Errorf("read release-gate fixture: %w", err)
	}
	digest := sha256.Sum256(fixtureBlob)
	if hex.EncodeToString(digest[:]) != corpus.Fixture.SHA256 {
		return ReleaseGateCorpus{}, fmt.Errorf("release-gate fixture digest mismatch")
	}
	sourceBlob, err := releaseGateFiles.ReadFile(corpus.SourcesFixture.Path)
	if err != nil {
		return ReleaseGateCorpus{}, fmt.Errorf("read release source fixture: %w", err)
	}
	sourceDigest := sha256.Sum256(sourceBlob)
	if hex.EncodeToString(sourceDigest[:]) != corpus.SourcesFixture.SHA256 {
		return ReleaseGateCorpus{}, fmt.Errorf("release source fixture digest mismatch")
	}
	var fixture certificationCatalogFixture
	if err := json.Unmarshal(fixtureBlob, &fixture); err != nil {
		return ReleaseGateCorpus{}, fmt.Errorf("decode release-gate fixture: %w", err)
	}
	fixtureCases := make(map[string]bool, len(fixture.Cases))
	for _, c := range fixture.Cases {
		fixtureCases[c.ID] = true
	}
	seen := make(map[string]bool, len(corpus.Cases))
	for _, c := range corpus.Cases {
		if c.ID == "" || c.Description == "" || (c.Lane != "live" && c.Lane != "hermetic") {
			return ReleaseGateCorpus{}, fmt.Errorf("release-gate case identity is invalid: %+v", c)
		}
		if seen[c.ID] {
			return ReleaseGateCorpus{}, fmt.Errorf("release-gate case %q is duplicated", c.ID)
		}
		seen[c.ID] = true
		if !fixtureCases[c.FixtureCase] {
			return ReleaseGateCorpus{}, fmt.Errorf("release-gate case %q references missing fixture case %q", c.ID, c.FixtureCase)
		}
		if c.MinAcceptableKeys < 0 || c.MinAcceptableKeys > len(c.AcceptableKeys) {
			return ReleaseGateCorpus{}, fmt.Errorf("release-gate case %q has an invalid acceptable-member minimum", c.ID)
		}
		if (c.ReferenceURL == "") != (len(c.ReferenceTitles) == 0) {
			return ReleaseGateCorpus{}, fmt.Errorf("release-gate case %q must bind a reference URL and titles together", c.ID)
		}
		allKeys := append(append(append([]string{}, c.AcceptableKeys...), c.ForbidKeys...), c.RequireKeys...)
		if _, err := parseProvisionKeys(allKeys); err != nil {
			return ReleaseGateCorpus{}, fmt.Errorf("release-gate case %q: %w", c.ID, err)
		}
	}
	return corpus, nil
}

func parseProvisionKeys(values []string) ([]provision.Key, error) {
	keys := make([]provision.Key, 0, len(values))
	for _, value := range values {
		key := provision.Key(value)
		if _, _, _, ok := provision.ParseKey(key); !ok {
			return nil, fmt.Errorf("invalid provisioning key %q", value)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func releaseGateCase(c ReleaseGateCorpusCase) (Case, error) {
	acceptable, err := parseProvisionKeys(c.AcceptableKeys)
	if err != nil {
		return Case{}, err
	}
	forbidden, err := parseProvisionKeys(c.ForbidKeys)
	if err != nil {
		return Case{}, err
	}
	required, err := parseProvisionKeys(c.RequireKeys)
	if err != nil {
		return Case{}, err
	}
	return Case{
		Name: c.ID, Intent: Intent{Description: c.Description}, NoFabrication: true,
		MinGrounded: c.MinGrounded, RequireKeys: required, ForbidKeys: forbidden,
		AcceptableKeys: acceptable, MinAcceptableKeys: c.MinAcceptableKeys,
		ExpectGroundedCompletion: c.Lane == "live", ExpectEraFrom: c.ExpectEraFrom, ExpectEraTo: c.ExpectEraTo,
		MaxToolCalls:          suggest.ProductionBounds().MaxToolCalls,
		MaxCandidatesSurfaced: suggest.ProductionBounds().MaxCandidatesSurfaced,
	}, nil
}

func ReleaseGateCase(id string) (Case, error) {
	corpus, err := LoadEmbeddedReleaseGateCorpus()
	if err != nil {
		return Case{}, err
	}
	for _, frozen := range corpus.Cases {
		if frozen.ID == id {
			return releaseGateCase(frozen)
		}
	}
	return Case{}, fmt.Errorf("release-gate case %q is missing", id)
}

func ReleaseGateLiveCases() ([]Case, error) {
	corpus, err := LoadEmbeddedReleaseGateCorpus()
	if err != nil {
		return nil, err
	}
	result := make([]Case, 0, len(corpus.Cases))
	for _, frozen := range corpus.Cases {
		if frozen.Lane != "live" {
			continue
		}
		c, err := releaseGateCase(frozen)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}

// ReleaseGateModelCanaryCases returns the fixed one-trial elimination subset.
// It intentionally reuses release-holdout cases instead of creating a weaker
// model-specific corpus.
func ReleaseGateModelCanaryCases() ([]Case, error) {
	cases := make([]Case, 0, len(releaseGateModelCanaryCaseIDs))
	for _, id := range releaseGateModelCanaryCaseIDs {
		c, err := ReleaseGateCase(id)
		if err != nil {
			return nil, err
		}
		if !c.ExpectGroundedCompletion {
			return nil, fmt.Errorf("release-gate model canary case %q is not live", id)
		}
		cases = append(cases, c)
	}
	return cases, nil
}

func ReleaseGateRunnerConfig(config RunnerConfig) (RunnerConfig, error) {
	corpus, err := LoadEmbeddedReleaseGateCorpus()
	if err != nil {
		return RunnerConfig{}, err
	}
	if strings.TrimSpace(config.Generator.Provider) == "" || buildScorecardRunSnapshot(Scorecard{
		CorpusVersion: corpus.Version, GeneratedAt: time.Unix(1, 0).UTC(),
		Profile: config.Profile, Generator: config.Generator,
	}, false) == nil {
		return RunnerConfig{}, fmt.Errorf("release scorecard requires identifier-shaped profile and generator provider/model")
	}
	config.Contract = &CertificationContract{
		CorpusVersion: corpus.Version, CatalogFixtureSHA256: corpus.Fixture.SHA256,
		PromptVersion: corpus.PromptVersion, ToolSchemaVersion: corpus.ToolSchemaVersion,
		ScorerVersion: corpus.ScorerVersion, HardMetrics: append([]string(nil), corpus.HardMetrics...),
		QualityMetrics: append([]string(nil), corpus.QualityMetrics...), Thresholds: corpus.Thresholds,
	}
	return config, nil
}

type releaseGateGenerator struct {
	inner        *suggest.Suggester
	fixture      *embeddedCatalogFixture
	caseByIntent map[string]string
	mu           sync.RWMutex
	selected     int
	surfaced     int
}

func (g *releaseGateGenerator) Suggest(ctx context.Context, intent suggest.Intent) (suggest.Proposal, error) {
	caseID, ok := g.caseByIntent[intent.Description]
	if !ok {
		return suggest.Proposal{}, fmt.Errorf("Intent is absent from the embedded release-gate corpus")
	}
	g.fixture.selectCase(caseID)
	proposal, err := g.inner.Suggest(ctx, intent)
	g.mu.Lock()
	g.selected = len(proposal.Lineup) + len(proposal.Acquisitions)
	g.surfaced = proposal.Trace.SurfacedTotal
	g.mu.Unlock()
	return proposal, err
}

func (g *releaseGateGenerator) begin() {
	g.mu.Lock()
	g.selected = 0
	g.surfaced = 0
	g.mu.Unlock()
}

func (g *releaseGateGenerator) counts() (int, int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.selected, g.surfaced
}

type releaseGateObserver struct {
	provider  *observedProvider
	fixture   *embeddedCatalogFixture
	generator *releaseGateGenerator
}

func (o *releaseGateObserver) Begin() {
	o.provider.Begin()
	o.fixture.beginMetrics()
	o.generator.begin()
}

func (o *releaseGateObserver) beginResourceRun(limits ResourceBudget, run, suite *resourceAccumulator) {
	o.provider.beginResourceRun(limits, run, suite)
}

func (o *releaseGateObserver) Snapshot(err error) Observation {
	observation := o.provider.Snapshot(err)
	providerSurfaced := observation.CandidatesSurfaced
	observation.CatalogOperations, observation.CatalogLatencyNanos = o.fixture.metrics()
	observation.SelectedCount, observation.CandidatesSurfaced = o.generator.counts()
	if providerSurfaced > observation.CandidatesSurfaced {
		observation.CandidatesSurfaced = providerSurfaced
	}
	return observation
}

func NewEmbeddedReleaseGateGenerator(provider llm.Provider) (Generator, Observer, error) {
	corpus, err := LoadEmbeddedReleaseGateCorpus()
	if err != nil {
		return nil, nil, err
	}
	fixtureBlob, err := releaseGateFiles.ReadFile(corpus.Fixture.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read release-gate fixture: %w", err)
	}
	var decoded certificationCatalogFixture
	if err := json.Unmarshal(fixtureBlob, &decoded); err != nil {
		return nil, nil, fmt.Errorf("decode release-gate fixture: %w", err)
	}
	fixture := newEmbeddedCatalogFixture(decoded)
	observed := &observedProvider{inner: provider}
	cat := catalog.New(embeddedLibraryCatalog{fixture}, embeddedTMDBCatalog{fixture}).WithPresence(fixture)
	sourceBlob, err := releaseGateFiles.ReadFile(corpus.SourcesFixture.Path)
	if err != nil {
		return nil, nil, err
	}
	references := &testkit.ReferenceResolver{ByURL: make(map[string]reference.Evidence)}
	if err := json.Unmarshal(sourceBlob, &references.ByLabel); err != nil {
		return nil, nil, fmt.Errorf("decode release source fixture: %w", err)
	}
	caseByIntent := make(map[string]string, len(corpus.Cases))
	for _, c := range corpus.Cases {
		caseByIntent[c.Description] = c.FixtureCase
		if len(c.ReferenceTitles) > 0 {
			references.ByURL[c.ReferenceURL] = reference.Evidence{URL: c.ReferenceURL, Title: "Frozen programming reference", Excerpt: "Synthetic release-gate evidence.", TitleAnchors: append([]string(nil), c.ReferenceTitles...)}
		}
	}
	suggester := suggest.New(observed, cat, fixture, 10).WithRatings(fixture).WithReferences(references)
	generator := &releaseGateGenerator{inner: suggester, fixture: fixture, caseByIntent: caseByIntent}
	return generator, &releaseGateObserver{provider: observed, fixture: fixture, generator: generator}, nil
}
