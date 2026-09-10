//go:build eval

package eval

import (
	"context"
	"os"
	"testing"
	"time"
)

const releaseGateTrials = 10
const modelCanaryTrials = 1
const modelFinalistTrials = 5

// TestPlannerReleaseGate is the explicit, non-CI release-intent replay. It uses
// the production Suggester against a digest-pinned private-data-free catalog;
// only the configured provider is live. No release target invokes this test.
func TestPlannerReleaseGate(t *testing.T) {
	if os.Getenv("LOOMARR_EVAL_PLANNER_RELEASE_GATE") != "1" {
		t.Skip("planner release gate is an explicit local command")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("planner release gate is local-only and must not run in CI")
	}
	if os.Getenv("LOOMARR_EVAL_CONTRACT_ONLY") == "1" {
		t.Fatal("planner release gate cannot run in contract-only mode")
	}
	if raw := os.Getenv("LOOMARR_EVAL_TRIALS"); raw != "" && raw != "10" {
		t.Fatal("planner release gate requires exactly 10 trials per frozen live intent")
	}
	cases, err := ReleaseGateLiveCases()
	if err != nil {
		t.Fatal(err)
	}
	runPlannerReleaseReplay(t, cases, releaseGateTrials, 30*time.Minute)
}

// TestPlannerModelCanary is the bounded elimination pass before repeated
// release replay. It is evidence about one configured model, not certification.
func TestPlannerModelCanary(t *testing.T) {
	if os.Getenv("LOOMARR_EVAL_PLANNER_MODEL_CANARY") != "1" {
		t.Skip("planner model canary is an explicit local command")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("planner model canary is local-only and must not run in CI")
	}
	if os.Getenv("LOOMARR_EVAL_CONTRACT_ONLY") == "1" {
		t.Fatal("planner model canary cannot run in contract-only mode")
	}
	if raw := os.Getenv("LOOMARR_EVAL_TRIALS"); raw != "" && raw != "1" {
		t.Fatal("planner model canary requires exactly one trial per frozen live intent")
	}
	cases, err := ReleaseGateModelCanaryCases()
	if err != nil {
		t.Fatal(err)
	}
	runPlannerReleaseReplay(t, cases, modelCanaryTrials, 90*time.Second)
}

// TestPlannerModelFinalist repeats only a canary winner under the separately
// authorized 1,000-call envelope. It still does not replace certification.
func TestPlannerModelFinalist(t *testing.T) {
	if os.Getenv("LOOMARR_EVAL_PLANNER_MODEL_FINALIST") != "1" {
		t.Skip("planner model finalist gate is an explicit local command")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("planner model finalist gate is local-only and must not run in CI")
	}
	if os.Getenv("LOOMARR_EVAL_CONTRACT_ONLY") == "1" {
		t.Fatal("planner model finalist gate cannot run in contract-only mode")
	}
	if raw := os.Getenv("LOOMARR_EVAL_TRIALS"); raw != "" && raw != "5" {
		t.Fatal("planner model finalist gate requires exactly five trials per frozen live intent")
	}
	cases, err := ReleaseGateModelCanaryCases()
	if err != nil {
		t.Fatal(err)
	}
	runPlannerReleaseReplay(t, cases, modelFinalistTrials, 6*time.Minute)
}

func runPlannerReleaseReplay(t *testing.T, cases []Case, trials int, timeout time.Duration) {
	t.Helper()
	generatorConfig, judgeConfig := certificationRoleConfigsFromEnv()
	profile := os.Getenv("LOOMARR_EVAL_PROFILE")
	generatorIdentity, judgeIdentity := CertificationIdentitiesFromEnv()
	budget, err := PrepareCertificationRun(len(cases), CertificationOptions{
		Required: true, FrozenCatalog: true, Trials: trials,
		GeneratorProvider: generatorConfig.Provider, GeneratorBaseURL: generatorConfig.BaseURL, GeneratorModel: generatorIdentity.Model,
		JudgeProvider: judgeConfig.Provider, JudgeBaseURL: judgeConfig.BaseURL, JudgeModel: judgeIdentity.Model,
		GeneratorUpstream: os.Getenv("LOOMARR_EVAL_GENERATOR_UPSTREAM_PROVIDER"),
		JudgeUpstream:     os.Getenv("LOOMARR_EVAL_JUDGE_UPSTREAM_PROVIDER"),
		AllowLocal:        os.Getenv("LOOMARR_EVAL_ALLOW_LOCAL") == "1",
		MaxCallsPerRun:    os.Getenv("LOOMARR_EVAL_MAX_CALLS_PER_RUN"), MaxCallsPerSuite: os.Getenv("LOOMARR_EVAL_MAX_CALLS_PER_SUITE"),
		MaxTokensPerRun: os.Getenv("LOOMARR_EVAL_MAX_TOKENS_PER_RUN"), MaxSpendPerRun: os.Getenv("LOOMARR_EVAL_MAX_SPEND_PER_RUN"),
		MaxTokensPerSuite: os.Getenv("LOOMARR_EVAL_MAX_TOKENS"), MaxSpendPerSuite: os.Getenv("LOOMARR_EVAL_MAX_SPEND"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pre-provider call budget: cases=%d trials=%d generator<=%d judge<=%d total<=%d declared_run<=%d declared_suite<=%d",
		budget.Cases, budget.Trials, budget.MaxGeneratorCalls, budget.MaxJudgeCalls, budget.Total,
		budget.Resource.MaxCallsPerRun, budget.Resource.MaxCallsPerSuite)
	if os.Getenv("LOOMARR_EVAL_OUT") == "" || os.Getenv("LOOMARR_EVAL_SUMMARY_OUT") == "" {
		t.Fatal("LOOMARR_EVAL_OUT and LOOMARR_EVAL_SUMMARY_OUT are required")
	}
	provider, err := NewCertificationProvider(generatorConfig)
	if err != nil {
		t.Fatal(err)
	}
	generator, observer, err := NewEmbeddedReleaseGateGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	config, err := ReleaseGateRunnerConfig(RunnerConfig{
		Trials: trials, Profile: profile,
		Generator: generatorIdentity, Judge: judgeIdentity, ResourceBudget: budget.Resource,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	runner := NewRunner(generator, config).WithObserver(observer)
	if normalizedProviderIdentity(generatorConfig.Provider) == "ollama" {
		runner = runner.WithResourceProbe(NewOllamaResourceProbe(generatorConfig.BaseURL))
	}
	card := runner.Run(ctx, cases)
	for _, result := range card.Results {
		for _, failure := range result.Failures {
			t.Errorf("%s trial %d: %s", result.Case, result.Trial, failure)
		}
	}
	if card.Assessment != nil {
		for _, failure := range card.Assessment.Failures {
			t.Errorf("release threshold: %s", failure)
		}
	}
	writeScorecard(t, card, true)
}
