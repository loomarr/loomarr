//go:build eval

package eval

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPrepareCertificationRunRequiresPinnedOpenRouterRoleRoutes(t *testing.T) {
	base := withRequiredResourceBudget(CertificationOptions{
		Required: true, LiveSchedule: true, Trials: 1,
		GeneratorProvider: "openrouter", JudgeProvider: "openrouter",
		GeneratorBaseURL: OpenRouterCertificationBaseURL, JudgeBaseURL: OpenRouterCertificationBaseURL,
		GeneratorModel: "openai/gpt-5-mini-2026-08-07", JudgeModel: "anthropic/claude-sonnet-4-20250514",
	})
	for name, mutate := range map[string]func(*CertificationOptions){
		"missing generator route": func(o *CertificationOptions) {
			o.JudgeUpstream = "Anthropic"
		},
		"missing judge route": func(o *CertificationOptions) {
			o.GeneratorUpstream = "OpenAI"
		},
		"malformed route": func(o *CertificationOptions) {
			o.GeneratorUpstream = "OpenAI,,Azure"
			o.JudgeUpstream = "Anthropic"
		},
		"multiple generator routes": func(o *CertificationOptions) {
			o.GeneratorUpstream = "OpenAI,Azure"
			o.JudgeUpstream = "Anthropic"
		},
	} {
		t.Run(name, func(t *testing.T) {
			options := base
			mutate(&options)
			if _, err := PrepareCertificationRun(1, options); err == nil || !strings.Contains(err.Error(), "provider") {
				t.Fatalf("required OpenRouter certification route error = %v", err)
			}
		})
	}
}

func TestPrepareCertificationRunRequiresCanonicalOpenRouterRoleURLs(t *testing.T) {
	base := withRequiredResourceBudget(CertificationOptions{
		Required: true, LiveSchedule: true, Trials: 1,
		GeneratorProvider: "openrouter", JudgeProvider: "openrouter",
		GeneratorBaseURL: OpenRouterCertificationBaseURL, JudgeBaseURL: OpenRouterCertificationBaseURL,
		GeneratorModel: "openai/gpt-5-mini-2026-08-07", GeneratorUpstream: "OpenAI",
		JudgeModel: "anthropic/claude-sonnet-4-20250514", JudgeUpstream: "Anthropic",
	})
	for name, mutate := range map[string]func(*CertificationOptions){
		"blank generator":     func(o *CertificationOptions) { o.GeneratorBaseURL = "" },
		"alternate generator": func(o *CertificationOptions) { o.GeneratorBaseURL = "https://example.invalid/v1" },
		"blank judge":         func(o *CertificationOptions) { o.JudgeBaseURL = "" },
		"alternate judge":     func(o *CertificationOptions) { o.JudgeBaseURL = OpenRouterCertificationBaseURL + "/" },
	} {
		t.Run(name, func(t *testing.T) {
			options := base
			mutate(&options)
			if _, err := PrepareCertificationRun(1, options); err == nil || !strings.Contains(err.Error(), "canonical OpenRouter API URL") {
				t.Fatalf("OpenRouter URL preflight error = %v", err)
			}
		})
	}

	custom := base
	custom.GeneratorProvider = "openai"
	custom.GeneratorBaseURL = "https://private-gateway.invalid/v1"
	if _, err := PrepareCertificationRun(1, custom); err != nil {
		t.Fatalf("generic OpenAI-compatible provider lost custom URL flexibility: %v", err)
	}
}

func TestPrepareCertificationRunRejectsMutableOpenRouterModelAliases(t *testing.T) {
	base := withRequiredResourceBudget(CertificationOptions{
		Required: true, LiveSchedule: true, Trials: 1,
		GeneratorBaseURL: OpenRouterCertificationBaseURL, JudgeBaseURL: OpenRouterCertificationBaseURL,
		GeneratorProvider: "openrouter", GeneratorModel: "openai/gpt-5-mini-2026-08-07", GeneratorUpstream: "OpenAI",
		JudgeProvider: "openrouter", JudgeModel: "anthropic/claude-sonnet-4-20250514", JudgeUpstream: "Anthropic",
	})
	for _, alias := range []string{"openrouter/auto", "openai/gpt-5-latest", "openai/gpt-5:free"} {
		options := base
		options.GeneratorModel = alias
		if _, err := PrepareCertificationRun(1, options); err == nil || !strings.Contains(err.Error(), "model") {
			t.Errorf("mutable model %q error = %v", alias, err)
		}
	}
}

func TestCertificationIdentitiesFromEnvKeepsJudgeProviderIndependentWhenModelFallsBack(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("LLM_MODEL", "openai/gpt-5-mini-2026-08-07")
	t.Setenv("LOOMARR_EVAL_JUDGE", "")
	t.Setenv("LOOMARR_EVAL_JUDGE_PROVIDER", "custom-judge")

	generator, judge := CertificationIdentitiesFromEnv()
	if generator != (ModelIdentity{Provider: "openrouter", Model: "openai/gpt-5-mini-2026-08-07"}) {
		t.Fatalf("generator identity = %+v", generator)
	}
	if judge != (ModelIdentity{Provider: "custom-judge", Model: "openai/gpt-5-mini-2026-08-07"}) {
		t.Fatalf("judge fallback identity = %+v", judge)
	}
	provider, err := buildJudgeProvider()
	if err != nil {
		t.Fatal(err)
	}
	if provider.Name() != "custom-judge" {
		t.Fatalf("judge provider builder identity = %q", provider.Name())
	}
	card := NewRunner(scriptedGenerator{proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix",
	}}}}, RunnerConfig{Generator: generator, Judge: judge}).Run(context.Background(), []Case{{Name: "identity"}})
	if card.Generator != generator || card.Judge != judge {
		t.Fatalf("scorecard env identities = generator %+v judge %+v", card.Generator, card.Judge)
	}
}

func TestCertificationProvidersSendPinnedOpenRouterRoutesAndPreserveWireAttribution(t *testing.T) {
	server := testkit.NewOpenRouter(
		`{"id":"gen-1","model":"openai/generator-resolved","choices":[{"message":{"role":"assistant","content":"{}"}}],"openrouter_metadata":{"attempt":1,"endpoints":{"available":[{"model":"openai/generator","provider":"OpenAI","selected":true}]}},"usage":{"prompt_tokens":11,"completion_tokens":2,"cost":0.0012300}}`,
		`{"id":"judge-1","model":"anthropic/judge-resolved","choices":[{"message":{"role":"assistant","content":"{\"overall\":0.9,\"relevance\":0.9,\"serendipity\":0.8,\"reason\":\"Grounded.\"}"}}],"openrouter_metadata":{"attempt":2,"endpoints":{"available":[{"model":"anthropic/judge","provider":"Anthropic","selected":true}]}},"usage":{"prompt_tokens":7,"completion_tokens":4,"cost":0.004200}}`,
	)
	t.Cleanup(server.Close)

	generator, err := NewCertificationProvider(CertificationProviderConfig{
		Provider: "openrouter", BaseURL: server.URL, Model: "openai/generator-2026-08-27",
		APIKey: "generator-secret", UpstreamProvider: "OpenAI",
	})
	if err != nil {
		t.Fatal(err)
	}
	judge, err := NewCertificationProvider(CertificationProviderConfig{
		Provider: "openrouter", BaseURL: server.URL, Model: "anthropic/judge-2026-08-27",
		APIKey: "judge-secret", UpstreamProvider: "Anthropic",
	})
	if err != nil {
		t.Fatal(err)
	}
	genResponse, err := generator.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "generate"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	judgeResponse, err := judge.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "judge"}}, llm.ChatOptions{JSONMode: true})
	if err != nil {
		t.Fatal(err)
	}

	requests := server.Requests()
	if len(requests) != 2 {
		t.Fatalf("captured OpenRouter requests = %d", len(requests))
	}
	for i, want := range [][]string{{"OpenAI"}, {"Anthropic"}} {
		route := requests[i].Provider
		if !reflect.DeepEqual(route.Order, want) || route.AllowFallbacks == nil || *route.AllowFallbacks ||
			route.RequireParameters == nil || !*route.RequireParameters || route.DataCollection != "deny" || route.ZDR == nil || !*route.ZDR {
			t.Errorf("request %d route = %+v, want pinned private route %v", i, route, want)
		}
	}
	if requests[0].Model != "openai/generator-2026-08-27" || requests[1].Model != "anthropic/judge-2026-08-27" {
		t.Errorf("captured role models = %q/%q", requests[0].Model, requests[1].Model)
	}
	if got := genResponse.Attribution; got.RequestedProvider != "openrouter" || got.ResolvedProvider != "OpenAI" ||
		got.ResolvedModel != "openai/generator-resolved" || got.Charge == nil || got.Charge.Amount != "0.0012300" {
		t.Errorf("generator wire attribution = %+v", got)
	}
	if got := judgeResponse.Attribution; got.RequestedProvider != "openrouter" || got.ResolvedProvider != "Anthropic" ||
		got.ResolvedModel != "anthropic/judge-resolved" || got.Attempts != 2 || got.Charge == nil || got.Charge.Amount != "0.004200" {
		t.Errorf("judge wire attribution = %+v", got)
	}
}
