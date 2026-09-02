package recommend_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/recommend"
)

func TestRunnerCertifiesOnlyACompleteSuiteMeetingFrozenThresholds(t *testing.T) {
	corpus, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	outputs := []string{
		`{"concepts":[]}`,
		`{"concepts":[{"name":"Upbeat Comedy","intent":{"description":"Upbeat comedy favorites"},"evidenceIds":["library:genre:comedy","preference:tone:upbeat"]},{"name":"Upbeat Drama","intent":{"description":"Upbeat dramatic stories"},"evidenceIds":["library:genre:drama","preference:tone:upbeat"]}]}`,
		`{"concepts":[{"name":"Thriller Nights","intent":{"description":"Tense thriller stories"},"evidenceIds":["library:genre:thriller"]}]}`,
		`{"concepts":[{"name":"Family Animation Club","intent":{"description":"A family-safe animation channel"},"evidenceIds":["library:genre:animation","constraint:audience:family"]}]}`,
		`{"concepts":[{"name":"Christmas Family","intent":{"description":"Christmas family favorites"},"evidenceIds":["season:christmas","library:genre:family"]}]}`,
		`{"concepts":[{"name":"1990s Science Fiction","intent":{"description":"1990s science-fiction stories"},"evidenceIds":["library:era:1990s","library:genre:science-fiction"]}]}`,
		`{"concepts":[]}`,
		`{"concepts":[{"name":"Mystery Draft","intent":{"description":"Draft-only mystery programming"},"evidenceIds":["library:genre:mystery","constraint:authority:draft-only"]}]}`,
	}
	provider := &scriptedProvider{name: "ollama"}
	for _, output := range outputs {
		provider.responses = append(provider.responses, llm.Response{Content: output, Attribution: llm.Attribution{
			RequestedProvider: "ollama", RequestedModel: "fixture:1b", Tokens: llm.TokenUsage{Prompt: 20, Completion: 10}, Attempts: 1,
		}})
	}
	runner, err := recommend.NewRunner(provider, recommend.RunConfig{
		Profile: "local-fixture", Model: "fixture:1b", ExpectedCases: 8, MaxCalls: 8, MaxTokens: 1_000,
		MaxSpendNanoUSD: 1, MaxOutputTokens: 256, PerCaseTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	card := runner.Run(context.Background(), corpus)
	if !card.Certified || card.StopReason != "" || len(card.Results) != 8 {
		t.Fatalf("certification = %+v", card)
	}
	if card.Quality.Relevance != 1 || card.Quality.SchemaValidity != 1 || len(card.HardFailureCounts) != 0 {
		t.Fatalf("aggregate quality = %+v hard failures %v", card.Quality, card.HardFailureCounts)
	}
	summary := recommend.HumanSummary(card)
	for _, want := range []string{"CERTIFIED", "channel-recommendation-v1", "fixture:1b", "8 calls", "$0.000000000", "Relevance"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q:\n%s", want, summary)
		}
	}
}

type scriptedProvider struct {
	name      string
	responses []llm.Response
	calls     int
	messages  [][]llm.Message
	options   []llm.ChatOptions
}

func (p *scriptedProvider) Name() string { return p.name }

func (p *scriptedProvider) Chat(_ context.Context, messages []llm.Message, options llm.ChatOptions) (llm.Response, error) {
	p.messages = append(p.messages, messages)
	p.options = append(p.options, options)
	response := p.responses[p.calls]
	p.calls++
	return response, nil
}

func TestRunnerUsesOneJSONOnlyCallPerSyntheticSnapshot(t *testing.T) {
	corpus, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{name: "ollama"}
	for range corpus.Cases {
		provider.responses = append(provider.responses, llm.Response{
			Content: `{"concepts":[]}`,
			Attribution: llm.Attribution{
				RequestedProvider: "ollama", RequestedModel: "fixture:1b",
				Tokens: llm.TokenUsage{Prompt: 20, Completion: 3}, Attempts: 1,
			},
		})
	}
	runner, err := recommend.NewRunner(provider, recommend.RunConfig{
		Profile: "local-fixture", Model: "fixture:1b", ExpectedCases: 8, MaxCalls: 8, MaxTokens: 1_000,
		MaxSpendNanoUSD: 1, MaxOutputTokens: 256, PerCaseTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	card := runner.Run(context.Background(), corpus)
	if provider.calls != len(corpus.Cases) || card.Resources.Calls != len(corpus.Cases) {
		t.Fatalf("calls = provider %d scorecard %d", provider.calls, card.Resources.Calls)
	}
	for i, messages := range provider.messages {
		if len(messages) != 2 || messages[0].Role != llm.System || messages[1].Role != llm.User {
			t.Fatalf("messages[%d] = %+v", i, messages)
		}
		if !provider.options[i].JSONMode || len(provider.options[i].Tools) != 0 || provider.options[i].MaxTokens != 256 {
			t.Fatalf("options[%d] = %+v", i, provider.options[i])
		}
		for _, forbidden := range []string{"userId", "viewerHistory", "requiredIntentTerms", "allowAbstention"} {
			if strings.Contains(messages[1].Content, forbidden) {
				t.Errorf("prompt[%d] leaked %q", i, forbidden)
			}
		}
	}
}

func TestRunnerFailsClosedWhenHostedAccountingIsMissing(t *testing.T) {
	corpus, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{name: "openrouter", responses: []llm.Response{{
		Content: `{"concepts":[]}`,
		Attribution: llm.Attribution{
			RequestedProvider: "openrouter", RequestedModel: "vendor/model-1",
			Tokens: llm.TokenUsage{Prompt: 20, Completion: 3}, Attempts: 1,
		},
	}}}
	runner, err := recommend.NewRunner(provider, recommend.RunConfig{
		Profile: "hosted-fixture", Model: "vendor/model-1", ExpectedCases: 8, MaxCalls: 8, MaxTokens: 1_000,
		MaxSpendNanoUSD: 1_000_000, MaxOutputTokens: 256, PerCaseTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	card := runner.Run(context.Background(), corpus)
	if card.Certified || provider.calls != 1 || card.StopReason != recommend.StopAccountingUnknown || card.Resources.AccountingComplete {
		t.Fatalf("hosted missing accounting = calls %d card %+v", provider.calls, card)
	}
}

func TestRunnerStopsWhenExactProviderChargeCrossesSuiteCeiling(t *testing.T) {
	corpus, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedProvider{name: "openrouter", responses: []llm.Response{{
		Content: `{"concepts":[]}`,
		Attribution: llm.Attribution{
			RequestedProvider: "openrouter", RequestedModel: "vendor/model-1",
			Tokens: llm.TokenUsage{Prompt: 20, Completion: 3}, Attempts: 1,
			Charge: &llm.Money{Amount: "0.000000011", Currency: "USD"},
		},
	}}}
	runner, err := recommend.NewRunner(provider, recommend.RunConfig{
		Profile: "hosted-fixture", Model: "vendor/model-1", ExpectedCases: 8, MaxCalls: 8, MaxTokens: 1_000,
		MaxSpendNanoUSD: 10, MaxOutputTokens: 256, PerCaseTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	card := runner.Run(context.Background(), corpus)
	if card.Certified || provider.calls != 1 || card.Resources.SpendNanoUSD != 11 || card.StopReason != recommend.StopSpendExceeded {
		t.Fatalf("spend stop = calls %d card %+v", provider.calls, card)
	}
}

func TestNewRunnerRejectsBudgetThatCannotCoverTheDeclaredSuite(t *testing.T) {
	provider := &scriptedProvider{name: "ollama"}
	_, err := recommend.NewRunner(provider, recommend.RunConfig{
		Profile: "local", Model: "fixture", ExpectedCases: 8, MaxCalls: 7, MaxTokens: 100,
		MaxSpendNanoUSD: 1, MaxOutputTokens: 10, PerCaseTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "max calls") {
		t.Fatalf("preflight error = %v", err)
	}
}
