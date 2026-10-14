//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const queryExpansionDiagnosticProfile = "query-expansion-diagnostic-v1"

type queryExpansionDiagnosticPreflight struct {
	SchemaVersion       int                                 `json:"schemaVersion"`
	InferenceAuthorized bool                                `json:"inferenceAuthorized"`
	Profile             string                              `json:"profile"`
	Generator           ModelIdentity                       `json:"generator"`
	UpstreamProvider    string                              `json:"upstreamProvider"`
	SnapshotSHA256      string                              `json:"snapshotSha256"`
	Selections          []QueryExpansionDiagnosticSelection `json:"selections"`
	Contract            *CertificationContract              `json:"contract"`
	CallBudget          CallBudget                          `json:"callBudget"`
	StructuralDemand    ResourceUsage                       `json:"structuralDemand"`
	ResourceMayStopRun  bool                                `json:"resourceMayStopRun"`
}

// TestQueryExpansionOpenRouterDiagnostic is an explicit local development
// diagnostic. Dry-run mode proves the complete route and resource contract
// without constructing a provider. Live mode additionally requires a separate
// authorization latch before the provider exists.
func TestQueryExpansionOpenRouterDiagnostic(t *testing.T) {
	if os.Getenv("LOOMARR_EVAL_QUERY_EXPANSION_DIAGNOSTIC") != "1" {
		t.Skip("query-expansion diagnostic is an explicit local command")
	}
	if os.Getenv("CI") != "" {
		t.Fatal("query-expansion diagnostic is local-only and must not run in CI")
	}
	if os.Getenv("LOOMARR_EVAL_CONTRACT_ONLY") == "1" {
		t.Fatal("query-expansion diagnostic cannot run in contract-only mode")
	}
	if raw := os.Getenv("LOOMARR_EVAL_TRIALS"); raw != "" && raw != "1" {
		t.Fatal("query-expansion diagnostic requires exactly one trial per selected behavior")
	}

	cases, err := QueryExpansionDiagnosticCases()
	if err != nil {
		t.Fatal(err)
	}
	generatorConfig, _ := certificationRoleConfigsFromEnv()
	if normalizedProviderIdentity(generatorConfig.Provider) != "openrouter" {
		t.Fatal("query-expansion diagnostic requires the strict OpenRouter provider")
	}
	generatorIdentity := ModelIdentity{Provider: "openrouter", Model: generatorConfig.Model}
	// The development cases have no subjective judge rubric. The same route is
	// nevertheless reserved as the structural judge worst case so the declared
	// 250-call envelope remains complete.
	judgeConfig := generatorConfig
	judgeIdentity := generatorIdentity
	generatorReservation, judgeReservation, err := openRouterReservationsFromEnv(
		os.Getenv, generatorConfig, judgeConfig, time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := PrepareCertificationRun(len(cases), CertificationOptions{
		Required: true, FrozenCatalog: true, Trials: 1,
		GeneratorProvider: generatorConfig.Provider, GeneratorBaseURL: generatorConfig.BaseURL, GeneratorModel: generatorIdentity.Model,
		JudgeProvider: judgeConfig.Provider, JudgeBaseURL: judgeConfig.BaseURL, JudgeModel: judgeIdentity.Model,
		GeneratorUpstream: generatorConfig.UpstreamProvider, JudgeUpstream: judgeConfig.UpstreamProvider,
		MaxCallsPerRun: os.Getenv("LOOMARR_EVAL_MAX_CALLS_PER_RUN"), MaxCallsPerSuite: os.Getenv("LOOMARR_EVAL_MAX_CALLS_PER_SUITE"),
		MaxTokensPerRun: os.Getenv("LOOMARR_EVAL_MAX_TOKENS_PER_RUN"), MaxSpendPerRun: os.Getenv("LOOMARR_EVAL_MAX_SPEND_PER_RUN"),
		MaxTokensPerSuite: os.Getenv("LOOMARR_EVAL_MAX_TOKENS"), MaxSpendPerSuite: os.Getenv("LOOMARR_EVAL_MAX_SPEND"),
		GeneratorOpenRouterReservation: generatorReservation,
		JudgeOpenRouterReservation:     judgeReservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	structuralDemand, resourceMayStopRun, err := queryExpansionDiagnosticStructuralDemand(budget)
	if err != nil {
		t.Fatal(err)
	}
	runnerConfig, err := QueryExpansionRunnerConfig(RunnerConfig{
		Trials: 1, Profile: queryExpansionDiagnosticProfile,
		Generator: generatorIdentity, Judge: judgeIdentity, ResourceBudget: budget.Resource,
		GeneratorReservation: budget.GeneratorReservation, JudgeReservation: budget.JudgeReservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	dryRun := os.Getenv("LOOMARR_EVAL_QUERY_EXPANSION_DRY_RUN") == "1"
	authorized := !dryRun && os.Getenv("LOOMARR_EVAL_QUERY_EXPANSION_AUTHORIZED") == "1"
	preflight := queryExpansionDiagnosticPreflight{
		SchemaVersion: 1, InferenceAuthorized: authorized,
		Profile: queryExpansionDiagnosticProfile, Generator: generatorIdentity,
		UpstreamProvider: generatorConfig.UpstreamProvider,
		SnapshotSHA256:   strings.TrimSpace(os.Getenv(openRouterSnapshotSHAEnv)),
		Selections:       QueryExpansionDiagnosticSelections(),
		Contract:         runnerConfig.Contract, CallBudget: budget,
		StructuralDemand: structuralDemand, ResourceMayStopRun: resourceMayStopRun,
	}
	preflightPath := strings.TrimSpace(os.Getenv("LOOMARR_EVAL_QUERY_EXPANSION_PREFLIGHT_OUT"))
	if preflightPath == "" {
		t.Fatal("LOOMARR_EVAL_QUERY_EXPANSION_PREFLIGHT_OUT is required")
	}
	if err := writeQueryExpansionDiagnosticPreflight(preflightPath, preflight); err != nil {
		t.Fatal(err)
	}
	t.Logf("query-expansion preflight: cases=%d generator<=%d judge<=%d total<=%d; structural demand<=%d tokens/USD %s; enforced suite ceilings=%d tokens/USD %s; may-stop=%t authorized=%t",
		budget.Cases, budget.MaxGeneratorCalls, budget.MaxJudgeCalls, budget.Total,
		structuralDemand.Tokens, structuralDemand.Spend,
		budget.Resource.MaxTokensPerSuite, budget.Resource.MaxSpendPerSuite, resourceMayStopRun, authorized)
	if dryRun {
		return
	}
	if !authorized {
		t.Fatal("live query-expansion inference requires LOOMARR_EVAL_QUERY_EXPANSION_AUTHORIZED=1")
	}
	if strings.TrimSpace(generatorConfig.APIKey) == "" {
		t.Fatal("OPENROUTER_API_KEY must be mapped to LLM_API_KEY for the live query-expansion diagnostic")
	}
	if os.Getenv("LOOMARR_EVAL_OUT") == "" || os.Getenv("LOOMARR_EVAL_SUMMARY_OUT") == "" {
		t.Fatal("LOOMARR_EVAL_OUT and LOOMARR_EVAL_SUMMARY_OUT are required")
	}
	provider, err := NewCertificationProvider(generatorConfig)
	if err != nil {
		t.Fatal(err)
	}
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	card := NewRunner(generator, runnerConfig).WithObserver(observer).Run(ctx, cases)
	for _, result := range card.Results {
		for _, failure := range result.Failures {
			t.Errorf("%s trial %d: %s", result.Case, result.Trial, failure)
		}
	}
	writeScorecardWithSummary(t, card, true, QueryExpansionDiagnosticSummary)
}

func queryExpansionDiagnosticStructuralDemand(budget CallBudget) (ResourceUsage, bool, error) {
	generatorTokens, ok := checkedMultiply(budget.MaxGeneratorCalls, budget.GeneratorReservation.Tokens)
	if !ok {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion generator token demand overflows")
	}
	judgeTokens, ok := checkedMultiply(budget.MaxJudgeCalls, budget.JudgeReservation.Tokens)
	if !ok {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion judge token demand overflows")
	}
	tokens, ok := checkedAdd(generatorTokens, judgeTokens)
	if !ok {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion suite token demand overflows")
	}
	calls, ok := checkedAdd(budget.MaxGeneratorCalls, budget.MaxJudgeCalls)
	if !ok {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion suite call demand overflows")
	}
	generatorSpend, valid := parseExactDecimal(budget.GeneratorReservation.Spend)
	if !valid {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion generator spend reservation is invalid")
	}
	judgeSpend, valid := parseExactDecimal(budget.JudgeReservation.Spend)
	if !valid {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion judge spend reservation is invalid")
	}
	spend := zeroDecimal()
	for range budget.MaxGeneratorCalls {
		spend = spend.add(generatorSpend)
	}
	for range budget.MaxJudgeCalls {
		spend = spend.add(judgeSpend)
	}
	maximumSpend, valid := parseExactDecimal(budget.Resource.MaxSpendPerSuite)
	if !valid {
		return ResourceUsage{}, false, fmt.Errorf("query-expansion suite spend ceiling is invalid")
	}
	demand := ResourceUsage{Calls: calls, Tokens: tokens, Spend: spend.String()}
	return demand, tokens > budget.Resource.MaxTokensPerSuite || spend.cmp(maximumSpend) > 0, nil
}

func writeQueryExpansionDiagnosticPreflight(path string, preflight queryExpansionDiagnosticPreflight) error {
	blob, err := json.MarshalIndent(preflight, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal query-expansion diagnostic preflight: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve query-expansion diagnostic preflight path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return fmt.Errorf("create query-expansion diagnostic preflight directory: %w", err)
	}
	if err := os.WriteFile(absolute, append(blob, '\n'), 0o644); err != nil {
		return fmt.Errorf("write query-expansion diagnostic preflight: %w", err)
	}
	return nil
}
