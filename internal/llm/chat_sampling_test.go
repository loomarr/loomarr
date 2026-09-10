package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

func TestOpenRouterGroundedSamplingPreservesExecutionControls(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		profile     llm.ChatProfile
		fixed       bool
	}{
		{"grounded fixed sampling", "google/gemini-3.8-flash", llm.GroundedSelection, true},
		{"other task", "google/gemini-3.8-flash", "", false},
		{"unknown model", "google/gemini-future", llm.GroundedSelection, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var sent map[string]any
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Error(err)
					return
				}
				if tc.fixed {
					if _, ok := sent["temperature"]; ok {
						t.Error("unsupported temperature excludes the Vertex route")
					}
					reasoning, _ := sent["reasoning"].(map[string]any)
					if reasoning["effort"] != "low" {
						t.Errorf("reasoning=%v, want low", reasoning)
					}
				} else if sent["temperature"] != 0.2 || sent["reasoning"] != nil {
					t.Errorf("unselected sampling changed: %v", sent)
				}
				route, _ := sent["provider"].(map[string]any)
				order, _ := route["order"].([]any)
				if len(order) != 1 || order[0] != "google-vertex/global" || route["allow_fallbacks"] != false || route["require_parameters"] != true || route["zdr"] != true || route["data_collection"] != "deny" {
					t.Errorf("strict route changed: %v", route)
				}
				if sent["max_tokens"] != float64(2048) || sent["tools"] == nil {
					t.Error("execution controls missing")
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
			}))
			defer server.Close()
			p, err := llm.NewOpenRouterChat(llm.OpenRouterChatConfig{BaseURL: server.URL, Model: tc.model, UpstreamProvider: "google-vertex/global"})
			if err != nil {
				t.Fatal(err)
			}
			temp := 0.2
			_, err = p.Chat(context.Background(), nil, llm.ChatOptions{Profile: tc.profile, Temperature: &temp, MaxTokens: 2048, Tools: []llm.ToolSchema{{Name: "catalog_search", Parameters: map[string]any{"type": "object"}}}})
			if err != nil {
				t.Fatal(err)
			}
			if temp != 0.2 {
				t.Fatal("adapter mutated caller sampling")
			}
		})
	}
}

func TestSolGroundedProfileBindsSupportedLimitAndPrivateRoute(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary client", true: "explicit route"}[explicit], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var sent map[string]any
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Error(err)
					return
				}
				grounded := calls == 0
				calls++
				if grounded {
					if sent["max_completion_tokens"] != float64(2048) || sent["max_tokens"] != nil || sent["temperature"] != nil {
						t.Errorf("unsupported grounded parameters: %v", sent)
					}
					reason, _ := sent["reasoning"].(map[string]any)
					if reason["effort"] != "none" {
						t.Errorf("reasoning = %v", reason)
					}
				} else if sent["max_tokens"] != float64(2048) || sent["max_completion_tokens"] != nil || sent["temperature"] != 0.2 || sent["reasoning"] != nil {
					t.Errorf("unselected task changed: %v", sent)
				}
				if grounded || explicit {
					route, _ := sent["provider"].(map[string]any)
					order, _ := route["order"].([]any)
					want := "azure/us"
					if explicit {
						want = "azure/eu"
					}
					if len(order) != 1 || order[0] != want || route["allow_fallbacks"] != false || route["require_parameters"] != true || route["data_collection"] != "deny" || route["zdr"] != true {
						t.Errorf("route lost its authority: %v", route)
					}
				} else if sent["provider"] != nil {
					t.Errorf("profile routing leaked to another task: %v", sent["provider"])
				}
				if sent["tools"] == nil {
					t.Error("tool definition lost")
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
			}))
			defer server.Close()
			p := llm.NewOpenAIForProvider("openrouter", server.URL, "openai/gpt-5.6-sol", "")
			if explicit {
				var err error
				p, err = llm.NewOpenRouterChat(llm.OpenRouterChatConfig{BaseURL: server.URL, Model: "openai/gpt-5.6-sol", UpstreamProvider: "azure/eu"})
				if err != nil {
					t.Fatal(err)
				}
			}
			temp := 0.2
			for _, profile := range []llm.ChatProfile{llm.GroundedSelection, ""} {
				_, err := p.Chat(context.Background(), nil, llm.ChatOptions{Profile: profile, Temperature: &temp, MaxTokens: 2048, Tools: []llm.ToolSchema{{Name: "catalog_search", Parameters: map[string]any{"type": "object"}}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			if temp != 0.2 {
				t.Fatal("caller sampling mutated")
			}
		})
	}
}

func TestSolGroundedProfileDoesNotChangeOtherProvidersOrModels(t *testing.T) {
	temp := 0.2
	for _, tc := range []struct{ provider, model string }{{"openai", "openai/gpt-5.6-sol"}, {"openrouter", "openai/gpt-future"}} {
		got := llm.ResolveChatPolicy(tc.provider, tc.model, llm.ChatOptions{Profile: llm.GroundedSelection, Temperature: &temp})
		if got.Temperature != &temp || got.ReasoningEffort != "" || got.DefaultUpstream != "" || got.CompletionLimitParameter != "max_tokens" {
			t.Errorf("unmatched capability changed: %+v", got)
		}
	}
}
