package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/recommend"
)

func TestRunWritesContentFreeDevelopmentArtifact(t *testing.T) {
	temp := t.TempDir()
	outPath := filepath.Join(temp, "diagnostic.json")
	provider := &fixtureProvider{}
	for range 5 {
		provider.responses = append(provider.responses, llm.Response{
			Content: `{"concepts":[]}`,
			Attribution: llm.Attribution{
				RequestedProvider: "ollama", RequestedModel: "fixture:1b",
				Tokens: llm.TokenUsage{Prompt: 20, Completion: 3}, Attempts: 1,
			},
		})
	}
	factoryCalls := 0
	factory := func(providerConfig) (llm.Provider, error) {
		factoryCalls++
		return provider, nil
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--provider", "ollama", "--model", "fixture:1b", "--profile", "fixture-development",
		"--artifact-digest", "0123456789ab", "--protocol", "json-mode-v1",
		"--max-calls", "5", "--max-tokens", "1000", "--max-spend-nanousd", "1",
		"--max-output-tokens", "64", "--out", outPath,
	}, func(string) string { return "" }, &stdout, &stderr, factory)
	if code != 0 || factoryCalls != 1 {
		t.Fatalf("run = %d factory calls %d stderr %s", code, factoryCalls, stderr.String())
	}
	blob, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var card recommend.DevelopmentScorecard
	if err := json.Unmarshal(blob, &card); err != nil {
		t.Fatal(err)
	}
	if !card.Completed || card.Protocol != recommend.ProtocolJSONModeV1 || card.Resources.Calls != 5 {
		t.Fatalf("artifact = %+v", card)
	}
	if stdout.String() == "" || stderr.String() != "" {
		t.Fatalf("stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestRunRejectsMissingCeilingBeforeProviderConstruction(t *testing.T) {
	factoryCalls := 0
	factory := func(providerConfig) (llm.Provider, error) {
		factoryCalls++
		return &fixtureProvider{}, nil
	}
	var stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--provider", "ollama", "--model", "fixture:1b", "--profile", "fixture-development",
		"--artifact-digest", "0123456789ab", "--protocol", "json-mode-v1",
		"--max-calls", "5", "--max-tokens", "1000", "--out", filepath.Join(t.TempDir(), "out.json"),
	}, func(string) string { return "" }, &bytes.Buffer{}, &stderr, factory)
	if code != 2 || factoryCalls != 0 {
		t.Fatalf("run = %d factory calls %d stderr %s", code, factoryCalls, stderr.String())
	}
}

type fixtureProvider struct {
	responses []llm.Response
	calls     int
}

func (provider *fixtureProvider) Name() string { return "ollama" }

func (provider *fixtureProvider) Chat(context.Context, []llm.Message, llm.ChatOptions) (llm.Response, error) {
	response := provider.responses[provider.calls]
	provider.calls++
	return response, nil
}
