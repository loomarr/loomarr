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

// recommend-cert runs the explicit, bounded, non-CI channel recommendation certification suite and
// writes machine and human scorecards.
const recommendCertName = "eval recommend-cert"

func init() {
	register("recommend-cert", "certify inert channel concepts on the frozen recommendation corpus (inference-spending)",
		func(args []string) int {
			return runRecommendCert(context.Background(), args, os.Getenv, os.Stdout, os.Stderr, recommendProvider("certification"))
		})
}

func runRecommendCert(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, factory providerFactory) int {
	flags := flag.NewFlagSet(recommendCertName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	providerName := flags.String("provider", envOr(getenv, "LLM_PROVIDER", "ollama"), "provider identity")
	baseURL := flags.String("base-url", getenv("LLM_URL"), "provider API base")
	model := flags.String("model", getenv("LLM_MODEL"), "exact model identity")
	profile := flags.String("profile", getenv("LOOMARR_RECOMMEND_PROFILE"), "certification profile identity")
	artifactDigest := flags.String("artifact-digest", getenv("LOOMARR_RECOMMEND_MODEL_DIGEST"), "local model artifact digest")
	upstream := flags.String("upstream-provider", getenv("LOOMARR_EVAL_GENERATOR_UPSTREAM_PROVIDER"), "pinned OpenRouter upstream")
	maxCalls := flags.String("max-calls", getenv("LOOMARR_RECOMMEND_MAX_CALLS"), "suite call ceiling")
	maxTokens := flags.String("max-tokens", getenv("LOOMARR_RECOMMEND_MAX_TOKENS"), "suite token ceiling")
	maxSpend := flags.String("max-spend-nanousd", getenv("LOOMARR_RECOMMEND_MAX_SPEND_NANOUSD"), "suite spend ceiling in nanodollars")
	maxOutputTokens := flags.Int("max-output-tokens", 0, "per-case output-token ceiling; omitted uses the frozen corpus contract")
	caseTimeout := flags.Duration("case-timeout", 2*time.Minute, "per-case wall-clock ceiling")
	dryRun := flags.Bool("dry-run", false, "write the provider-free certification contract without inference")
	outPath := flags.String("out", getenv("LOOMARR_RECOMMEND_OUT"), "machine scorecard path")
	summaryPath := flags.String("summary", getenv("LOOMARR_RECOMMEND_SUMMARY_OUT"), "human summary path")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	corpus, err := recommend.LoadCorpus()
	if err != nil {
		return commandError(stderr, recommendCertName, "load corpus", err)
	}
	resolvedMaxOutputTokens := *maxOutputTokens
	if resolvedMaxOutputTokens == 0 {
		resolvedMaxOutputTokens = corpus.MaxOutputTokens
	}
	if resolvedMaxOutputTokens <= 0 || resolvedMaxOutputTokens != corpus.MaxOutputTokens {
		report(stderr, "%s: --max-output-tokens must equal frozen %s ceiling %d\n", recommendCertName, corpus.Version, corpus.MaxOutputTokens)
		return 2
	}
	parsedCalls, parsedTokens, parsedSpend, err := parseCeilings(*maxCalls, *maxTokens, *maxSpend)
	if err != nil {
		report(stderr, "%s: %v\n", recommendCertName, err)
		return 2
	}
	if strings.TrimSpace(*model) == "" || strings.TrimSpace(*profile) == "" {
		report(stderr, "%s: --model and --profile are required\n", recommendCertName)
		return 2
	}
	if !*dryRun && (strings.TrimSpace(*outPath) == "" || strings.TrimSpace(*summaryPath) == "") {
		report(stderr, "%s: --out and --summary are required for live certification\n", recommendCertName)
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
	if !*dryRun && provider == "openrouter" && apiKey == "" {
		report(stderr, "%s: OPENROUTER_API_KEY or LLM_API_KEY is required\n", recommendCertName)
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
		ExpectedCases: len(corpus.Cases), MaxCalls: parsedCalls,
		MaxTokens: parsedTokens, MaxSpendNanoUSD: parsedSpend, MaxOutputTokens: resolvedMaxOutputTokens,
		PerCaseTimeout: *caseTimeout,
	}
	if parsedCalls < len(corpus.Cases) {
		report(stderr, "%s: max calls %d cannot cover %d declared cases\n", recommendCertName, parsedCalls, len(corpus.Cases))
		return 2
	}
	if *dryRun {
		contract, buildErr := recommend.BuildCertificationContract(provider, corpus, config)
		if buildErr != nil {
			return commandUsageError(stderr, recommendCertName, "build certification contract", buildErr)
		}
		blob, marshalErr := json.MarshalIndent(contract, "", "  ")
		if marshalErr != nil {
			return commandError(stderr, recommendCertName, "encode certification contract", marshalErr)
		}
		_, _ = stdout.Write(append(blob, '\n'))
		return 0
	}
	generator, err := factory(providerConfig{
		Provider: provider, BaseURL: url, Model: *model, APIKey: apiKey, Upstream: *upstream,
	})
	if err != nil {
		return commandUsageError(stderr, recommendCertName, "construct provider", err)
	}
	runner, err := recommend.NewRunner(generator, config)
	if err != nil {
		return commandUsageError(stderr, recommendCertName, "configure runner", err)
	}
	card := runner.Run(ctx, corpus)
	jsonBlob, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return commandError(stderr, recommendCertName, "encode scorecard", err)
	}
	jsonBlob = append(jsonBlob, '\n')
	summary := recommend.HumanSummary(card)
	if err := writeArtifact(*outPath, jsonBlob); err != nil {
		return commandError(stderr, recommendCertName, "write machine scorecard", err)
	}
	if err := writeArtifact(*summaryPath, []byte(summary)); err != nil {
		return commandError(stderr, recommendCertName, "write human summary", err)
	}
	_, _ = fmt.Fprint(stdout, summary)
	if !card.Certified {
		return 1
	}
	return 0
}
