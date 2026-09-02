//go:build eval

package eval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestEmbeddedCertificationCorpusIsFrozenHeldOutAndRepresentative(t *testing.T) {
	corpus, err := LoadEmbeddedCertificationCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "planner-certification-v2" {
		t.Fatalf("corpus version = %q, want planner-certification-v2", corpus.Version)
	}
	if corpus.SchemaVersion != 2 {
		t.Fatalf("corpus schema version = %d, want 2", corpus.SchemaVersion)
	}
	if corpus.Split != "certification" {
		t.Fatalf("corpus split = %q, want certification", corpus.Split)
	}
	if len(corpus.Cases) != 25 {
		t.Fatalf("certification scenario families = %d, want 25", len(corpus.Cases))
	}
	if corpus.Fixture.SHA256 == "" {
		t.Fatal("catalog fixture digest is empty")
	}

	seen := make(map[string]bool, len(corpus.Cases))
	axes := make(map[string]bool)
	for _, c := range corpus.Cases {
		if c.ID == "" || seen[c.ID] {
			t.Fatalf("case id %q is blank or duplicated", c.ID)
		}
		seen[c.ID] = true
		if c.Split != corpus.Split {
			t.Fatalf("case %q split = %q, want %q", c.ID, c.Split, corpus.Split)
		}
		if len(c.Variants) != 5 {
			t.Fatalf("case family %q variants = %d, want 5 plus its base Intent", c.ID, len(c.Variants))
		}
		for _, axis := range c.Axes {
			axes[axis] = true
		}
	}
	for _, axis := range []string{
		"tool-routing", "must-include", "must-exclude", "refinement",
		"season-window", "audience-ceiling", "ambiguous", "conflicting",
		"thin-results", "empty-results", "tool-error", "repair-turn",
		"adversarial-fabrication",
	} {
		if !axes[axis] {
			t.Errorf("certification corpus does not cover %q", axis)
		}
	}
	if slices.Contains(corpus.AllowedTrainingSplits, corpus.Split) {
		t.Fatalf("certification split appears in allowed training splits: %v", corpus.AllowedTrainingSplits)
	}
	if corpus.Thresholds.MinGroundedCompletionRate != 0.95 ||
		corpus.Thresholds.MinCorrectToolOperationRate != 0.90 ||
		corpus.Thresholds.MinSchemaValidityRate != 0.98 || corpus.Thresholds.MaxP95ToolCalls != 3 {
		t.Fatalf("certification thresholds drifted: %+v", corpus.Thresholds)
	}
}

func TestOllamaResourceProbeReportsResidentModelRAMAndVRAM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			t.Fatalf("resource probe path = %q, want /api/ps", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma-fixture:9b","size":8589934592,"size_vram":6442450944}]}`))
	}))
	defer server.Close()

	measurement := NewOllamaResourceProbe(server.URL).Measure(context.Background(), ModelIdentity{Model: "gemma-fixture:9b"})
	if measurement.Status != "measured" || measurement.PeakRAMBytes != 2<<30 || measurement.PeakVRAMBytes != 6<<30 {
		t.Fatalf("Ollama resource measurement = %+v", measurement)
	}
}

func TestCertificationScorecardCarriesVersionedContractAndHumanSummary(t *testing.T) {
	config, err := CertificationRunnerConfig(RunnerConfig{
		Generator: ModelIdentity{Provider: "ollama", Model: "fixture-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(scriptedGenerator{}, config).Run(context.Background(), []Case{{Name: "safe", NoFabrication: true}})
	if card.Contract == nil || card.Contract.CatalogFixtureSHA256 == "" || card.CorpusVersion != "planner-certification-v2" {
		t.Fatalf("scorecard certification contract = %+v", card)
	}
	summary := HumanSummary(card)
	for _, want := range []string{"fixture-model", "Hard gates", "Quality metrics", "1/1 trials passed"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("human summary missing %q:\n%s", want, summary)
		}
	}
}

func TestRunnerExecutesCertificationCaseAgainstPinnedCatalogFixture(t *testing.T) {
	provider := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "Synthetic Matrix"}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":10001,"name":"Synthetic Matrix"}]}`),
	)
	generator, observer, err := NewEmbeddedCertificationGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, RunnerConfig{}).WithObserver(observer).Run(context.Background(), cases[:1])
	if !card.Certified || len(card.Results) != 1 || !card.Results[0].Passed() {
		t.Fatalf("fixture-backed certification result = %+v", card)
	}
	if card.Results[0].ToolCalls != 1 || card.Results[0].CandidatesSurfaced != 1 {
		t.Fatalf("fixture observation = %+v, want one tool call and one surfaced candidate", card.Results[0].Observation)
	}
}

func TestRunnerScoresWrongCertificationToolRouteAsQuality(t *testing.T) {
	runner := NewRunner(scriptedGenerator{}, RunnerConfig{Contract: &CertificationContract{
		Thresholds: CertificationThresholds{MinCorrectToolOperationRate: 1},
	}}).WithObserver(&scriptedObserver{
		value: Observation{ToolCalls: 1, GenreCalls: 1, GroundingStage: "grounded"},
	})
	card := runner.Run(context.Background(), []Case{{
		Name: "named-title", NoFabrication: true, ExpectedToolOperation: "title",
	}})
	if card.Results[0].FailureStage != "" || !card.Results[0].Passed() {
		t.Fatalf("quality miss became a hard failure: %+v", card.Results[0])
	}
	if card.Certified || card.Assessment == nil || card.Assessment.CorrectToolOperationRate != 0 {
		t.Fatalf("wrong tool route escaped aggregate threshold: %+v", card)
	}
}

func TestCertificationAbstentionPassCarriesNoFailureStage(t *testing.T) {
	runner := NewRunner(scriptedGenerator{err: suggest.ErrNoGroundedTitles}, RunnerConfig{}).WithObserver(&scriptedObserver{
		value: Observation{ToolCalls: 1, KeywordCalls: 1, GroundingStage: "selection_empty"},
	})
	card := runner.Run(context.Background(), []Case{{Name: "empty", NoFabrication: true}})
	if !card.Results[0].Passed() || card.Results[0].FailureStage != "" {
		t.Fatalf("passing abstention retained a failure stage: %+v", card.Results[0])
	}
}

func TestFrozenPlannerCertificationDoesNotRequireLiveScheduleEvidence(t *testing.T) {
	options := withRequiredResourceBudget(CertificationOptions{
		Required: true, FrozenCatalog: true, Trials: 1, GeneratorProvider: "openai",
	})
	if _, err := PrepareCertificationRun(25, options); err != nil {
		t.Fatalf("frozen planner certification required live schedule evidence: %v", err)
	}
}

type scriptedResourceProbe struct{ measurement ResourceMeasurement }

func (p scriptedResourceProbe) Measure(context.Context, ModelIdentity) ResourceMeasurement {
	return p.measurement
}

type sequenceObserver struct {
	values []Observation
	next   int
}

func (*sequenceObserver) Begin() {}

func (o *sequenceObserver) Snapshot(error) Observation {
	value := o.values[o.next]
	o.next++
	return value
}

func TestCertificationScorecardAggregatesLatencyToolCallsAndPeakMemory(t *testing.T) {
	observer := &sequenceObserver{values: []Observation{
		{ToolCalls: 1, GroundingStage: "grounded", generatorCalls: []InferenceCall{{LatencyNanos: 10}}},
		{ToolCalls: 2, GroundingStage: "grounded", generatorCalls: []InferenceCall{{LatencyNanos: 20}}},
		{ToolCalls: 2, GroundingStage: "grounded", generatorCalls: []InferenceCall{{LatencyNanos: 30}}},
		{ToolCalls: 3, GroundingStage: "grounded", generatorCalls: []InferenceCall{{LatencyNanos: 100}}},
	}}
	runner := NewRunner(scriptedGenerator{}, RunnerConfig{Contract: &CertificationContract{
		Thresholds: CertificationThresholds{MaxP95ToolCalls: 2},
	}}).WithObserver(observer).WithResourceProbe(scriptedResourceProbe{measurement: ResourceMeasurement{
		Status: "measured", Source: "fixture", PeakRAMBytes: 8 << 30, PeakVRAMBytes: 6 << 30,
	}})
	card := runner.Run(context.Background(), []Case{
		{Name: "p10", NoFabrication: true}, {Name: "p20", NoFabrication: true},
		{Name: "p30", NoFabrication: true}, {Name: "p100", NoFabrication: true},
	})
	if card.Assessment == nil {
		t.Fatal("certification assessment is missing")
	}
	performance := card.Assessment.Performance
	if performance.GeneratorLatencyP50Nanos != 20 || performance.GeneratorLatencyP95Nanos != 100 {
		t.Fatalf("run latency percentiles = %+v, want p50=20ns p95=100ns", performance)
	}
	if performance.P95ToolCalls != 3 || performance.PeakRAMBytes != 8<<30 || performance.PeakVRAMBytes != 6<<30 {
		t.Fatalf("performance evidence = %+v", performance)
	}
	if card.Certified || len(card.Assessment.Failures) != 1 {
		t.Fatalf("p95 tool-call threshold did not fail certification: %+v", card.Assessment)
	}
	summary := HumanSummary(card)
	for _, want := range []string{"Latency p50/p95: 20ns / 100ns", "Peak RAM/VRAM: 8.00 GiB / 6.00 GiB"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("performance summary missing %q:\n%s", want, summary)
		}
	}
}

func TestCertificationCasesAreExecutableAndHaveHardGates(t *testing.T) {
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 150 {
		t.Fatalf("executable certification cases = %d, want 150", len(cases))
	}
	abstentions := 0
	intents := make(map[string]bool, len(cases))
	for _, c := range cases {
		if c.Name == "" || c.Intent.Description == "" {
			t.Fatalf("case has blank executable identity or Intent: %+v", c)
		}
		if c.MaxToolCalls <= 0 || c.MaxCandidatesSurfaced <= 0 {
			t.Fatalf("case %q lacks production structural hard gates", c.Name)
		}
		if !c.NoFabrication {
			t.Fatalf("case %q does not hard-gate unsupported ids", c.Name)
		}
		if !c.ExpectGroundedCompletion {
			abstentions++
		}
		if intents[c.Intent.Description] {
			t.Fatalf("Intent %q is duplicated", c.Intent.Description)
		}
		intents[c.Intent.Description] = true
	}
	if abstentions != 18 {
		t.Fatalf("abstention cases = %d, want exactly 18 explicit empty/conflict cases", abstentions)
	}
}
