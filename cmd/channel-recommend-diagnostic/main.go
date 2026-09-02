package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/recommend"
)

const openRouterAPIBase = "https://openrouter.ai/api/v1"

type providerConfig struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
	Upstream string
}

type providerFactory func(providerConfig) (llm.Provider, error)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr, newProvider))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, factory providerFactory) int {
	flags := flag.NewFlagSet("channel-recommend-diagnostic", flag.ContinueOnError)
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
		_, _ = fmt.Fprintf(stderr, "channel-recommend-diagnostic: %v\n", err)
		return 2
	}
	selectedProtocol := recommend.DevelopmentProtocol(strings.TrimSpace(*protocol))
	if selectedProtocol != recommend.ProtocolJSONModeV1 && selectedProtocol != recommend.ProtocolPromptOnlyV1 {
		_, _ = fmt.Fprintln(stderr, "channel-recommend-diagnostic: --protocol must be json-mode-v1 or prompt-only-v1")
		return 2
	}
	corpus, err := recommend.LoadDevelopmentCorpus()
	if err != nil {
		return commandError(stderr, "load development corpus", err)
	}
	if strings.TrimSpace(*model) == "" || strings.TrimSpace(*profile) == "" || strings.TrimSpace(*outPath) == "" {
		_, _ = fmt.Fprintln(stderr, "channel-recommend-diagnostic: --model, --profile, and --out are required")
		return 2
	}
	if parsedCalls < len(corpus.Cases) {
		_, _ = fmt.Fprintf(stderr, "channel-recommend-diagnostic: max calls %d cannot cover %d declared cases\n", parsedCalls, len(corpus.Cases))
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
		_, _ = fmt.Fprintln(stderr, "channel-recommend-diagnostic: OPENROUTER_API_KEY or LLM_API_KEY is required")
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
		return commandUsageError(stderr, "construct provider", err)
	}
	runner, err := recommend.NewRunner(generator, config)
	if err != nil {
		return commandUsageError(stderr, "configure runner", err)
	}
	card := runner.RunDevelopment(ctx, corpus, selectedProtocol)
	blob, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return commandError(stderr, "encode diagnostic", err)
	}
	if err := writeArtifact(*outPath, append(blob, '\n')); err != nil {
		return commandError(stderr, "write diagnostic", err)
	}
	_, _ = fmt.Fprint(stdout, recommend.HumanDevelopmentSummary(card))
	if !card.Completed {
		return 1
	}
	return 0
}

func newProvider(config providerConfig) (llm.Provider, error) {
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}
	switch config.Provider {
	case "ollama":
		return llm.NewOllama(config.BaseURL, config.Model), nil
	case "openrouter":
		if config.BaseURL != openRouterAPIBase {
			return nil, fmt.Errorf("OpenRouter diagnosis requires canonical API base %s", openRouterAPIBase)
		}
		return llm.NewOpenRouterChat(llm.OpenRouterChatConfig{
			BaseURL: config.BaseURL, Model: config.Model, APIKey: config.APIKey, UpstreamProvider: config.Upstream,
		})
	default:
		if config.BaseURL == "" {
			return nil, fmt.Errorf("provider API base is required")
		}
		return llm.NewOpenAIForProvider(config.Provider, config.BaseURL, config.Model, config.APIKey), nil
	}
}

func parseCeilings(calls, tokens, spend string) (int, int, int64, error) {
	parsePositiveInt := func(name, raw string) (int, error) {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("--%s must be an explicit positive integer", name)
		}
		return value, nil
	}
	parsedCalls, err := parsePositiveInt("max-calls", calls)
	if err != nil {
		return 0, 0, 0, err
	}
	parsedTokens, err := parsePositiveInt("max-tokens", tokens)
	if err != nil {
		return 0, 0, 0, err
	}
	parsedSpend, err := strconv.ParseInt(spend, 10, 64)
	if err != nil || parsedSpend <= 0 {
		return 0, 0, 0, fmt.Errorf("--max-spend-nanousd must be an explicit positive integer")
	}
	return parsedCalls, parsedTokens, parsedSpend, nil
}

func writeArtifact(path string, blob []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".channel-recommend-diagnostic-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err = temporary.Write(blob); err == nil {
		err = temporary.Chmod(0o644)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func envOr(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}

func commandUsageError(stderr io.Writer, operation string, err error) int {
	_, _ = fmt.Fprintf(stderr, "channel-recommend-diagnostic: %s: %v\n", operation, err)
	return 2
}

func commandError(stderr io.Writer, operation string, err error) int {
	_, _ = fmt.Fprintf(stderr, "channel-recommend-diagnostic: %s: %v\n", operation, err)
	return 1
}
