package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

const diagnosticTimeout = 2 * time.Minute

type diagnosticRunner func(context.Context, llm.Provider, string) (suggest.ToolFinalizationDiagnostic, error)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), diagnosticTimeout)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, os.Stdout, os.Stderr, suggest.RunToolFinalizationDiagnostic))
}

func run(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer, execute diagnosticRunner) int {
	providerName := strings.ToLower(strings.TrimSpace(getenv("LLM_PROVIDER")))
	if providerName == "" {
		providerName = "ollama"
	}
	baseURL := strings.TrimSpace(getenv("LLM_URL"))
	model := strings.TrimSpace(getenv("LLM_MODEL"))
	if baseURL == "" || model == "" {
		_, _ = fmt.Fprintln(stderr, "planner-tool-diagnostic: LLM_URL and LLM_MODEL are required")
		return 2
	}

	provider, err := diagnosticProvider(providerName, baseURL, model, getenv("LLM_API_KEY"), getenv("LOOMARR_EVAL_GENERATOR_UPSTREAM_PROVIDER"))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-tool-diagnostic: %v\n", err)
		return 2
	}
	report, diagnosticErr := execute(ctx, provider, model)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_, _ = fmt.Fprintf(stderr, "planner-tool-diagnostic: encode report: %v\n", err)
		return 1
	}
	if diagnosticErr != nil {
		_, _ = fmt.Fprintf(stderr, "planner-tool-diagnostic: %v\n", diagnosticErr)
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
