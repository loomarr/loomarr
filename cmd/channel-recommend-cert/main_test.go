package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

type commandProvider struct{ calls int }

func (p *commandProvider) Name() string { return "ollama" }

func (p *commandProvider) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (llm.Response, error) {
	p.calls++
	return llm.Response{Content: `{"concepts":[]}`, Attribution: llm.Attribution{
		RequestedProvider: "ollama", RequestedModel: "fixture:1b", Tokens: llm.TokenUsage{Prompt: 10, Completion: 2}, Attempts: 1,
	}}, nil
}

func TestCommandRejectsMissingExplicitResourceCeilingsBeforeProviderConstruction(t *testing.T) {
	constructed := false
	code := run(context.Background(), []string{"--model", "fixture:1b", "--profile", "local"}, func(string) string { return "" },
		&bytes.Buffer{}, &bytes.Buffer{}, func(providerConfig) (llm.Provider, error) {
			constructed = true
			return &commandProvider{}, nil
		})
	if code != 2 || constructed {
		t.Fatalf("exit = %d, provider constructed = %t", code, constructed)
	}
}

func TestCommandWritesBothScorecardsEvenWhenModelDoesNotCertify(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "result.json")
	markdownPath := filepath.Join(dir, "result.md")
	provider := &commandProvider{}
	args := []string{
		"--model", "fixture:1b", "--profile", "local-fixture",
		"--max-calls", "8", "--max-tokens", "1000", "--max-spend-nanousd", "1",
		"--out", jsonPath, "--summary", markdownPath,
	}
	code := run(context.Background(), args, func(string) string { return "super-secret" },
		&bytes.Buffer{}, &bytes.Buffer{}, func(providerConfig) (llm.Provider, error) { return provider, nil })
	if code != 1 || provider.calls != 8 {
		t.Fatalf("exit = %d, calls = %d", code, provider.calls)
	}
	for _, path := range []string{jsonPath, markdownPath} {
		blob, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(blob) == 0 || strings.Contains(string(blob), "super-secret") {
			t.Fatalf("unsafe or empty artifact %s: %s", path, blob)
		}
	}
}
