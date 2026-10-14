package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

// Ollama is always self-hosted (it's Loomarr's own local wire); confirm that
// still flows onto Attribution after the SelfHosted field was added.
func TestOllama_ChatAttributionIsAlwaysSelfHosted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ok"}}`))
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "llama3.1:8b")
	resp, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Attribution.SelfHosted {
		t.Errorf("Attribution.SelfHosted = false, want true for ollama")
	}
}

// Attribution.SelfHosted must come from the endpoint's own identity
// (provider + host), not be inferred downstream from the provider name — a
// household running LLM_PROVIDER=openai against its own private/Tailscale
// server is still self-hosted, and a curated hosted brand omitting its
// charge must not be excused just because it happens to be reached locally
// in a test (#1852 checkpoint 2).
func TestOpenAI_ChatAttributionCarriesSelfHostedFromEndpointIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"gen-1","model":"served-model","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name     string
		provider string
		want     bool
	}{
		// The exact household shape the eval harness was wrongly latching
		// budget_exhausted on: LLM_PROVIDER=openai pointed at a private host.
		{"openai wire against a private host is self-hosted", "openai", true},
		{"openrouter is never self-hosted regardless of host", "openrouter", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := llm.NewOpenAIForProvider(tc.provider, srv.URL, "served-model", "")
			resp, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{MaxTokens: 16})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Attribution.SelfHosted != tc.want {
				t.Errorf("Attribution.SelfHosted = %v, want %v", resp.Attribution.SelfHosted, tc.want)
			}
		})
	}
}
