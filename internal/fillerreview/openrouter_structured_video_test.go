package fillerreview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallOpenRouterStructuredCarriesOneDataVideoOnPinnedRoute(t *testing.T) {
	var request openRouterStructuredRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "generation", "model": "review/video-model",
			"choices": []any{map[string]any{"message": map[string]any{"content": `{"ok":true}`, "reasoning": ""}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "cost": 0.001},
			"openrouter_metadata": map[string]any{
				"attempt":   1,
				"attempts":  []any{map[string]any{"provider": "Video Route", "model": "review/video-model-2026", "status": 200}},
				"endpoints": map[string]any{"available": []any{map[string]any{"provider": "Video Route", "model": "review/video-model-2026", "selected": true}}},
			},
		})
	}))
	defer server.Close()
	result, err := callOpenRouterStructured(t.Context(), server.Client(), server.URL, openRouterStructuredCallConfig{
		APIKey: "key", Model: "review/video-model", ResolvedModel: "review/video-model-2026",
		UpstreamProvider: "Video Route", ProviderSlug: "video/route", SchemaName: "video_test",
		Schema: map[string]any{"type": "object"}, SystemPrompt: "screen", Content: "one video",
		Videos:    []openRouterStructuredVideo{{MIMEType: "video/mp4", Base64: "YWJj"}},
		MaxTokens: 32, MaxChargeNanoUSD: 2_000_000, DisableReasoning: true, Title: "test",
		Reserve: func(string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := request.Messages[1].Content
	if result.StructuredOutput != `{"ok":true}` || len(parts) != 2 || parts[1].Type != "video_url" || parts[1].VideoURL == nil || parts[1].VideoURL.URL != "data:video/mp4;base64,YWJj" || request.Provider.AllowFallbacks || !request.Provider.ZDR || request.Provider.DataCollection != "deny" {
		t.Fatalf("result=%+v request=%+v", result, request)
	}
}

func TestCallOpenRouterStructuredRejectsInvalidVideoBeforeReservation(t *testing.T) {
	reserved := false
	_, err := callOpenRouterStructured(t.Context(), http.DefaultClient, "https://openrouter.ai/api/v1", openRouterStructuredCallConfig{
		Videos:  []openRouterStructuredVideo{{MIMEType: "application/octet-stream", Base64: "YWJj"}},
		Reserve: func(string) error { reserved = true; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "invalid MIME") || reserved {
		t.Fatalf("err=%v reserved=%t", err, reserved)
	}
}
