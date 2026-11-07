package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/platform/llm"
)

// The provider, ceiling and artifact plumbing shared by recommend-cert and recommend-diagnostic.

const openRouterAPIBase = "https://openrouter.ai/api/v1"

type providerConfig struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
	Upstream string
}

type providerFactory func(providerConfig) (llm.Provider, error)

// recommendProvider builds the live provider; purpose names the run in the OpenRouter base refusal.
func recommendProvider(purpose string) providerFactory {
	return func(config providerConfig) (llm.Provider, error) {
		if strings.TrimSpace(config.Model) == "" {
			return nil, fmt.Errorf("model is required")
		}
		switch config.Provider {
		case "ollama":
			return llm.NewOllama(config.BaseURL, config.Model), nil
		case "openrouter":
			if config.BaseURL != openRouterAPIBase {
				return nil, fmt.Errorf("OpenRouter %s requires canonical API base %s", purpose, openRouterAPIBase)
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

// writeArtifact replaces path atomically through a temporary file in the same directory.
func writeArtifact(path string, blob []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".channel-recommend-*")
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

func commandUsageError(stderr io.Writer, command, operation string, err error) int {
	report(stderr, "%s: %s: %v\n", command, operation, err)
	return 2
}

func commandError(stderr io.Writer, command, operation string, err error) int {
	if !errors.Is(err, context.Canceled) {
		report(stderr, "%s: %s: %v\n", command, operation, err)
	}
	return 1
}
