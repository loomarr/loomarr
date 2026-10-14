package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

type commandProvider struct {
	calls   int
	options []llm.ChatOptions
}

func TestCommandDryRunWritesFrozenContractWithoutProviderConstruction(t *testing.T) {
	constructed := false
	stdout := &bytes.Buffer{}
	args := []string{
		"--dry-run", "--model", "fixture:1b", "--profile", "local-fixture", "--artifact-digest", "0123456789ab",
		"--max-calls", "8", "--max-tokens", "10000", "--max-spend-nanousd", "1",
	}
	code := run(context.Background(), args, func(string) string { return "" },
		stdout, &bytes.Buffer{}, func(providerConfig) (llm.Provider, error) {
			constructed = true
			return &commandProvider{}, nil
		})
	if code != 0 || constructed {
		t.Fatalf("exit = %d, provider constructed = %t", code, constructed)
	}
	var contract struct {
		CorpusVersion       string `json:"corpusVersion"`
		CorpusSHA256        string `json:"corpusSha256"`
		InferenceAuthorized bool   `json:"inferenceAuthorized"`
		Budget              struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.CorpusVersion != "channel-recommendation-v2" || contract.CorpusSHA256 != "2caf971fd7ad14cc9c673c6c4bf92d481305086e9506d0fd67eb8f63cff1e17c" ||
		contract.InferenceAuthorized || contract.Budget.MaxOutputTokens != 1024 {
		t.Fatalf("contract = %+v", contract)
	}
}

func (p *commandProvider) Name() string { return "ollama" }

func (p *commandProvider) Chat(_ context.Context, _ []llm.Message, options llm.ChatOptions) (llm.Response, error) {
	p.calls++
	p.options = append(p.options, options)
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
		"--model", "fixture:1b", "--profile", "local-fixture", "--artifact-digest", "0123456789ab",
		"--max-calls", "8", "--max-tokens", "1000", "--max-spend-nanousd", "1",
		"--out", jsonPath, "--summary", markdownPath,
	}
	getenv := func(key string) string {
		if strings.Contains(key, "API_KEY") {
			return "super-secret"
		}
		return ""
	}
	code := run(context.Background(), args, getenv,
		&bytes.Buffer{}, &bytes.Buffer{}, func(providerConfig) (llm.Provider, error) { return provider, nil })
	if code != 1 || provider.calls != 8 {
		t.Fatalf("exit = %d, calls = %d", code, provider.calls)
	}
	for _, options := range provider.options {
		if !options.JSONMode || options.MaxTokens != 1024 {
			t.Fatalf("provider options = %+v", options)
		}
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

func TestCommandRejectsOutputCeilingDriftBeforeProviderConstruction(t *testing.T) {
	dir := t.TempDir()
	constructed := false
	args := []string{
		"--model", "fixture:1b", "--profile", "local-fixture", "--artifact-digest", "0123456789ab",
		"--max-calls", "8", "--max-tokens", "1000", "--max-spend-nanousd", "1", "--max-output-tokens", "512",
		"--out", filepath.Join(dir, "result.json"), "--summary", filepath.Join(dir, "result.md"),
	}
	code := run(context.Background(), args, func(string) string { return "" },
		&bytes.Buffer{}, &bytes.Buffer{}, func(providerConfig) (llm.Provider, error) {
			constructed = true
			return &commandProvider{}, nil
		})
	if code != 2 || constructed {
		t.Fatalf("exit = %d, provider constructed = %t", code, constructed)
	}
}
