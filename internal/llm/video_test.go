package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

func TestNewOpenRouterVideoRefusesUnadvertisedCapability(t *testing.T) {
	_, err := llm.NewOpenRouterVideo(llm.OpenRouterVideoConfig{
		ProviderKey:      "openrouter",
		Transport:        llm.VideoTransportBase64,
		BaseURL:          "https://openrouter.ai/api/v1",
		APIKey:           "unused",
		Model:            llm.HostedModel{ID: "openai/gpt-4.1-mini", Vision: true},
		UpstreamProvider: "OpenAI",
	})
	if err == nil {
		t.Fatal("image-only model must not acquire direct-video capability")
	}
}

func TestOpenRouterVideoSendsBoundedBase64OnPinnedPrivateRoute(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer secret" {
			t.Errorf("authorization = %q", auth)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"video-1","model":"google/gemma-4-26b-a4b-it","choices":[{"message":{"content":"{\"kind\":\"commercial\"}"}}],"usage":{"prompt_tokens":99,"completion_tokens":6,"cost":0.0042}}`))
	}))
	defer srv.Close()
	provider, err := llm.NewOpenRouterVideo(llm.OpenRouterVideoConfig{
		ProviderKey:      "openrouter",
		Transport:        llm.VideoTransportBase64,
		BaseURL:          srv.URL,
		APIKey:           "secret",
		Model:            llm.HostedModel{ID: "google/gemma-4-26b-a4b-it", Video: true},
		UpstreamProvider: "Google AI Studio",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := provider.AskAboutVideo(context.Background(), "classify only what is visible", llm.VideoInput{
		Data: []byte{0, 1, 2}, MIMEType: "video/mp4", DurationMS: 59_999,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != `{"kind":"commercial"}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.Attribution.ResolvedProvider != "" || resp.Attribution.Attempts != 0 || resp.Attribution.Tokens.Prompt != 99 || resp.Attribution.Charge == nil || resp.Attribution.Charge.Amount != "0.0042" {
		t.Fatalf("attribution = %+v", resp.Attribution)
	}

	want := map[string]any{
		"model":      "google/gemma-4-26b-a4b-it",
		"max_tokens": float64(512),
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "classify only what is visible"},
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "data:video/mp4;base64,AAEC"}},
			},
		}},
		"provider": map[string]any{
			"order":              []any{"Google AI Studio"},
			"allow_fallbacks":    false,
			"require_parameters": true,
			"data_collection":    "deny",
			"zdr":                true,
		},
	}
	if jsonString(got) != jsonString(want) {
		t.Fatalf("request = %s\nwant    = %s", jsonString(got), jsonString(want))
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestOpenRouterVideoRefusesUnsafeInputBeforeNetwork(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer srv.Close()
	provider, err := llm.NewOpenRouterVideo(llm.OpenRouterVideoConfig{
		ProviderKey:      "openrouter",
		Transport:        llm.VideoTransportBase64,
		BaseURL:          srv.URL,
		Model:            llm.HostedModel{ID: "google/gemini-video", Video: true},
		UpstreamProvider: "Google AI Studio",
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, input := range map[string]llm.VideoInput{
		"URL transport": {URL: "https://example.test/clip.mp4", MIMEType: "video/mp4", DurationMS: 1_000},
		"missing bytes": {MIMEType: "video/mp4", DurationMS: 1_000},
		"wrong format":  {Data: []byte("video"), MIMEType: "video/webm", DurationMS: 1_000},
		"duration":      {Data: []byte("video"), MIMEType: "video/mp4", DurationMS: 60_001},
		"size":          {Data: make([]byte, llm.MaxHostedVideoBytes+1), MIMEType: "video/mp4", DurationMS: 1_000},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := provider.AskAboutVideo(context.Background(), "classify", input); err == nil {
				t.Fatal("unsafe input must fail closed")
			}
		})
	}
	if requests != 0 {
		t.Fatalf("unsafe inputs made %d hosted requests, want zero", requests)
	}
}

func TestOpenRouterVideoRefusesOversizedProviderResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, llm.MaxHostedVideoResponseBytes+1))
	}))
	defer srv.Close()
	provider, err := llm.NewOpenRouterVideo(llm.OpenRouterVideoConfig{
		ProviderKey: "openrouter", Transport: llm.VideoTransportBase64, BaseURL: srv.URL, Model: llm.HostedModel{ID: "google/gemini-video", Video: true},
		UpstreamProvider: "Google AI Studio",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.AskAboutVideo(context.Background(), "classify", llm.VideoInput{
		Data: []byte("mp4"), MIMEType: "video/mp4", DurationMS: 1_000,
	})
	if err == nil {
		t.Fatal("oversized provider response must fail closed")
	}
}

func TestNewOpenRouterVideoRequiresPinnedConcreteRoute(t *testing.T) {
	base := llm.OpenRouterVideoConfig{
		ProviderKey:      "openrouter",
		Transport:        llm.VideoTransportBase64,
		BaseURL:          "https://openrouter.ai/api/v1",
		APIKey:           "unused",
		Model:            llm.HostedModel{ID: "google/gemini-video", Video: true},
		UpstreamProvider: "Google AI Studio",
	}
	for name, mutate := range map[string]func(*llm.OpenRouterVideoConfig){
		"missing API base":   func(c *llm.OpenRouterVideoConfig) { c.BaseURL = "" },
		"custom provider":    func(c *llm.OpenRouterVideoConfig) { c.ProviderKey = "custom" },
		"unknown transport":  func(c *llm.OpenRouterVideoConfig) { c.Transport = "url" },
		"moving model alias": func(c *llm.OpenRouterVideoConfig) { c.Model.ID = "google/gemini-latest" },
		"unnamespaced model": func(c *llm.OpenRouterVideoConfig) { c.Model.ID = "gemini-video" },
		"unpinned route":     func(c *llm.OpenRouterVideoConfig) { c.UpstreamProvider = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mutate(&cfg)
			if _, err := llm.NewOpenRouterVideo(cfg); err == nil {
				t.Fatal("unsafe capability configuration must fail closed")
			}
		})
	}
}
