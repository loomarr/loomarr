//go:build eval

package eval

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func TestEmbeddedCertificationCorpusIsFrozenHeldOutAndRepresentative(t *testing.T) {
	corpus, err := LoadEmbeddedCertificationCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "planner-certification-v1" {
		t.Fatalf("corpus version = %q, want planner-certification-v1", corpus.Version)
	}
	if corpus.Split != "certification" {
		t.Fatalf("corpus split = %q, want certification", corpus.Split)
	}
	if len(corpus.Cases) != 25 {
		t.Fatalf("certification cases = %d, want 25", len(corpus.Cases))
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
}

func TestCertificationScorecardCarriesVersionedContractAndHumanSummary(t *testing.T) {
	config, err := CertificationRunnerConfig(RunnerConfig{
		Generator: ModelIdentity{Provider: "ollama", Model: "fixture-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(scriptedGenerator{}, config).Run(context.Background(), []Case{{Name: "safe", NoFabrication: true}})
	if card.Contract == nil || card.Contract.CatalogFixtureSHA256 == "" || card.CorpusVersion != "planner-certification-v1" {
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

func TestRunnerRejectsWrongCertificationToolRoute(t *testing.T) {
	runner := NewRunner(scriptedGenerator{}, RunnerConfig{}).WithObserver(&scriptedObserver{
		value: Observation{ToolCalls: 1, GenreCalls: 1, GroundingStage: "grounded"},
	})
	card := runner.Run(context.Background(), []Case{{
		Name: "named-title", NoFabrication: true, ExpectedToolOperation: "title",
	}})
	if card.Certified || card.Results[0].FailureStage != FailureStageDeterministic {
		t.Fatalf("wrong tool route certified: %+v", card.Results[0])
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

func TestCertificationCasesAreExecutableAndHaveHardGates(t *testing.T) {
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 25 {
		t.Fatalf("executable certification cases = %d, want 25", len(cases))
	}
	abstentions := 0
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
		if c.MinGrounded == 0 {
			abstentions++
		}
	}
	if abstentions != 3 {
		t.Fatalf("abstention cases = %d, want exactly 3 explicit empty/conflict cases", abstentions)
	}
}
