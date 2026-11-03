package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/recommend"
)

// recommend-diagnostic runs a bounded, content-free protocol diagnosis against the synthetic
// recommendation development split.
const recommendDiagnosticName = "eval recommend-diagnostic"

func init() {
	register("recommend-diagnostic", "diagnose recommendation JSON transport on the development corpus (inference-spending)",
		func(args []string) int {
			return runRecommendDiagnostic(context.Background(), args, os.Getenv, os.Stdout, os.Stderr, recommendProvider("diagnosis"))
		})
}

func runRecommendDiagnostic(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, factory providerFactory) int {
	flags := flag.NewFlagSet(recommendDiagnosticName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	providerName := flags.String("provider", envOr(getenv, "LLM_PROVIDER", "ollama"), "provider identity")
	baseURL := flags.String("base-url", getenv("LLM_URL"), "provider API base")
	model := flags.String("model", getenv("LLM_MODEL"), "exact model identity")
	profile := flags.String("profile", getenv("LOOMARR_RECOMMEND_PROFILE"), "development profile identity")
	artifactDigest := flags.String("artifact-digest", getenv("LOOMARR_RECOMMEND_MODEL_DIGEST"), "local model artifact digest")
	upstream := flags.String("upstream-provider", getenv("LOOMARR_EVAL_GENERATOR_UPSTREAM_PROVIDER"), "pinned OpenRouter upstream")
	protocol := flags.String("protocol", getenv("LOOMARR_RECOMMEND_PROTOCOL"), "json-mode-v1 or prompt-only-v1")
	maxCalls := flags.String("max-calls", getenv("LOOMARR_RECOMMEND_MAX_CALLS"), "suite call ceiling")
	maxTokens := flags.String("max-tokens", getenv("LOOMARR_RECOMMEND_MAX_TOKENS"), "suite token ceiling")
	maxSpend := flags.String("max-spend-nanousd", getenv("LOOMARR_RECOMMEND_MAX_SPEND_NANOUSD"), "suite spend ceiling in nanodollars")
	maxOutputTokens := flags.Int("max-output-tokens", 512, "per-case output-token ceiling")
	caseTimeout := flags.Duration("case-timeout", 2*time.Minute, "per-case wall-clock ceiling")
	outPath := flags.String("out", getenv("LOOMARR_RECOMMEND_OUT"), "content-free diagnostic artifact path")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	parsedCalls, parsedTokens, parsedSpend, err := parseCeilings(*maxCalls, *maxTokens, *maxSpend)
	if err != nil {
		report(stderr, "%s: %v\n", recommendDiagnosticName, err)
		return 2
	}
	selectedProtocol := recommend.DevelopmentProtocol(strings.TrimSpace(*protocol))
	if selectedProtocol != recommend.ProtocolJSONModeV1 && selectedProtocol != recommend.ProtocolPromptOnlyV1 {
		report(stderr, "%s: --protocol must be json-mode-v1 or prompt-only-v1\n", recommendDiagnosticName)
		return 2
	}
	corpus, err := recommend.LoadDevelopmentCorpus()
	if err != nil {
		return commandError(stderr, recommendDiagnosticName, "load development corpus", err)
	}
	if strings.TrimSpace(*model) == "" || strings.TrimSpace(*profile) == "" || strings.TrimSpace(*outPath) == "" {
		report(stderr, "%s: --model, --profile, and --out are required\n", recommendDiagnosticName)
		return 2
	}
	if parsedCalls < len(corpus.Cases) {
		report(stderr, "%s: max calls %d cannot cover %d declared cases\n", recommendDiagnosticName, parsedCalls, len(corpus.Cases))
		return 2
	}

	provider := strings.ToLower(strings.TrimSpace(*providerName))
	url := strings.TrimSpace(*baseURL)
	if provider == "ollama" && url == "" {
		url = "http://127.0.0.1:11434"
	}
	apiKey := getenv("LLM_API_KEY")
	if apiKey == "" && provider == "openrouter" {
		apiKey = getenv("OPENROUTER_API_KEY")
	}
	if provider == "openrouter" && apiKey == "" {
		report(stderr, "%s: OPENROUTER_API_KEY or LLM_API_KEY is required\n", recommendDiagnosticName)
		return 2
	}
	recordedDigest, recordedUpstream := "", ""
	if provider == "ollama" {
		recordedDigest = *artifactDigest
	}
	if provider == "openrouter" {
		recordedUpstream = *upstream
	}
	config := recommend.RunConfig{
		Profile: *profile, Model: *model, ArtifactDigest: recordedDigest, Upstream: recordedUpstream,
		ExpectedCases: len(corpus.Cases), MaxCalls: parsedCalls, MaxTokens: parsedTokens,
		MaxSpendNanoUSD: parsedSpend, MaxOutputTokens: *maxOutputTokens, PerCaseTimeout: *caseTimeout,
	}
	generator, err := factory(providerConfig{Provider: provider, BaseURL: url, Model: *model, APIKey: apiKey, Upstream: *upstream})
	if err != nil {
		return commandUsageError(stderr, recommendDiagnosticName, "construct provider", err)
	}
	runner, err := recommend.NewRunner(generator, config)
	if err != nil {
		return commandUsageError(stderr, recommendDiagnosticName, "configure runner", err)
	}
	card := runner.RunDevelopment(ctx, corpus, selectedProtocol)
	blob, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return commandError(stderr, recommendDiagnosticName, "encode diagnostic", err)
	}
	if err := writeArtifact(*outPath, append(blob, '\n')); err != nil {
		return commandError(stderr, recommendDiagnosticName, "write diagnostic", err)
	}
	_, _ = fmt.Fprint(stdout, recommend.HumanDevelopmentSummary(card))
	if !card.Completed {
		return 1
	}
	return 0
}
