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
