package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

// planner-tool-diagnostic is the explicit, non-CI planner tool-finalization diagnostic. It performs
// one bounded model call against fixed synthetic data, configured only by LLM_* environment, and
// emits content-safe contract hashes and outcome metadata; it never logs prompt text, tool results,
// model output, credentials, or reasoning content.
const plannerToolDiagnosticName = "eval planner-tool-diagnostic"

const diagnosticTimeout = 2 * time.Minute

type diagnosticRunner func(context.Context, llm.Provider, string) (suggest.ToolFinalizationDiagnostic, error)

func init() {
	register("planner-tool-diagnostic", "probe the exact post-catalog-result model turn; LLM_* environment (inference-spending)",
		func([]string) int {
			ctx, cancel := context.WithTimeout(context.Background(), diagnosticTimeout)
			defer cancel()
			return runPlannerToolDiagnostic(ctx, os.Getenv, os.Stdout, os.Stderr, suggest.RunToolFinalizationDiagnostic)
		})
}

func runPlannerToolDiagnostic(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer, execute diagnosticRunner) int {
	providerName := strings.ToLower(strings.TrimSpace(getenv("LLM_PROVIDER")))
	if providerName == "" {
		providerName = "ollama"
	}
	baseURL := strings.TrimSpace(getenv("LLM_URL"))
	model := strings.TrimSpace(getenv("LLM_MODEL"))
	if baseURL == "" || model == "" {
		report(stderr, "%s: LLM_URL and LLM_MODEL are required\n", plannerToolDiagnosticName)
		return 2
	}

	provider, err := diagnosticProvider(providerName, baseURL, model, getenv("LLM_API_KEY"), getenv("LOOMARR_EVAL_GENERATOR_UPSTREAM_PROVIDER"))
	if err != nil {
		report(stderr, "%s: %v\n", plannerToolDiagnosticName, err)
		return 2
	}
	result, diagnosticErr := execute(ctx, provider, model)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		report(stderr, "%s: encode report: %v\n", plannerToolDiagnosticName, err)
		return 1
	}
	if diagnosticErr != nil {
		report(stderr, "%s: %v\n", plannerToolDiagnosticName, diagnosticErr)
		return 1
	}
	return 0
}

func diagnosticProvider(providerName, baseURL, model, apiKey, upstream string) (llm.Provider, error) {
	switch providerName {
	case "ollama":
		return llm.NewOllama(baseURL, model), nil
	case "openrouter":
		return llm.NewOpenRouterChat(llm.OpenRouterChatConfig{
			BaseURL: baseURL, Model: model, APIKey: apiKey, UpstreamProvider: upstream,
		})
	default:
		return llm.NewOpenAIForProvider(providerName, baseURL, model, apiKey), nil
	}
}
