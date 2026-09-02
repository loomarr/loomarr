//go:build eval

package eval

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestPlannerModelCertification is the explicit, inference-spending lane for
// comparing candidate models against the frozen catalog fixture. Family-smoke
// mode selects one canonical Intent per family and is not release certification.
func TestPlannerModelCertification(t *testing.T) {
	if os.Getenv("LOOMARR_EVAL_PLANNER_CERTIFICATION") != "1" {
		t.Skip("planner-model certification is an explicit non-CI command")
	}
	if os.Getenv("LOOMARR_EVAL_CONTRACT_ONLY") == "1" {
		t.Fatal("planner-model certification cannot run in contract-only mode")
	}
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("LOOMARR_EVAL_PLANNER_FAMILY_SMOKE") == "1" {
		cases, err = CertificationFamilySmokeCases()
		if err != nil {
			t.Fatal(err)
		}
	}
	trials, err := ParseEvaluationTrials(true, os.Getenv("LOOMARR_EVAL_TRIALS"))
	if err != nil {
		t.Fatal(err)
	}
	generatorConfig, judgeConfig := certificationRoleConfigsFromEnv()
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
	if os.Getenv("LOOMARR_EVAL_OUT") == "" || os.Getenv("LOOMARR_EVAL_SUMMARY_OUT") == "" {
		t.Fatal("LOOMARR_EVAL_OUT and LOOMARR_EVAL_SUMMARY_OUT are required")
	}
	provider, err := NewCertificationProvider(generatorConfig)
	if err != nil {
		t.Fatal(err)
	}
	generator, observer, err := NewEmbeddedCertificationGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	runnerConfig, err := CertificationRunnerConfig(RunnerConfig{
		Trials: trials, Profile: os.Getenv("LOOMARR_EVAL_PROFILE"),
		Generator: generatorIdentity, Judge: judgeIdentity, ResourceBudget: budget.Resource,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	runner := NewRunner(generator, runnerConfig).WithObserver(observer)
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
			t.Errorf("certification threshold: %s", failure)
		}
	}
	writeScorecard(t, card, true)
}
