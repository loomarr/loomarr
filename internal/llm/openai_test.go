package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
)

// The OpenAI provider normalizes the one shape difference from Ollama: tool-call
// `arguments` arrive as a JSON STRING and must be decoded to a map for the
// suggester. Also assert response_format (JSONMode), sampling, and Bearer auth.
func TestOpenAI_NormalizesToolCallArgumentsAndSampling(t *testing.T) {
	var sentReq map[string]any
	var authHdr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		// Respond with a tool call whose arguments are a JSON STRING (OpenAI shape).
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"",
			"tool_calls":[{"id":"call_1","type":"function","function":{
				"name":"catalog_search","arguments":"{\"query\":\"matrix\",\"genres\":[\"Action\"]}"}}]}}]}`))
	}))
	defer srv.Close()

	o := llm.NewOpenAI(srv.URL, "gpt-x", "sk-test")
	temp := 0.2
	resp, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}},
		llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search"}}, JSONMode: true, Temperature: &temp, MaxTokens: 256})
	if err != nil {
		t.Fatal(err)
	}

	// 1. The JSON-string arguments were decoded into a map the suggester can read.
	if !resp.WantsTools() || len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "catalog_search" {
		t.Errorf("tool name = %q", tc.Name)
	}
	if q, _ := tc.Arguments["query"].(string); q != "matrix" {
		t.Errorf("arguments not decoded to a map: %+v", tc.Arguments)
	}
	if _, ok := tc.Arguments["genres"].([]any); !ok {
		t.Errorf("nested arg (genres array) not decoded: %+v", tc.Arguments["genres"])
	}

	// 2. response_format sent on JSONMode (best-effort JSON hint).
	rf, _ := sentReq["response_format"].(map[string]any)
	if rf["type"] != "json_object" {
		t.Errorf("response_format not sent on JSONMode: %v", sentReq["response_format"])
	}
	// 3. sampling forwarded + Bearer auth.
	if sentReq["temperature"] != 0.2 {
		t.Errorf("temperature = %v, want 0.2", sentReq["temperature"])
	}
	if sentReq["max_tokens"] != float64(256) {
		t.Errorf("max_tokens = %v, want 256", sentReq["max_tokens"])
	}
	if authHdr != "Bearer sk-test" {
		t.Errorf("auth header = %q, want Bearer sk-test", authHdr)
	}
}

// Final content (no tool calls) round-trips as the Response content.
func TestOpenAI_FinalContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"picks\":[]}"}}]}`))
	}))
	defer srv.Close()

	o := llm.NewOpenAI(srv.URL, "m", "")
	resp, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.WantsTools() {
		t.Error("no tool calls expected")
	}
	if resp.Content != `{"picks":[]}` {
		t.Errorf("content = %q", resp.Content)
	}
}

func TestOpenAI_AttributesOpenRouterResolutionUsageAndExactCharge(t *testing.T) {
	var metadataHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadataHeader = r.Header.Get("X-OpenRouter-Metadata")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"gen-123",
			"model":"openai/gpt-5-mini-2026-08-07",
			"choices":[{"message":{"role":"assistant","content":"{\"role\":\"commercial\"}"}}],
			"openrouter_metadata":{
				"attempt":2,
				"endpoints":{"available":[
					{"model":"openai/gpt-5-mini","provider":"Azure","selected":false},
					{"model":"openai/gpt-5-mini","provider":"OpenAI","selected":true}
				]}
			},
			"usage":{
				"prompt_tokens":194,
				"completion_tokens":12,
				"prompt_tokens_details":{"cached_tokens":31,"cache_write_tokens":7,"audio_tokens":4},
				"completion_tokens_details":{"reasoning_tokens":5,"audio_tokens":2},
				"cost":0.0000012300
			}
		}`))
	}))
	defer srv.Close()

	o := llm.NewOpenAIForProvider("openrouter", srv.URL, "openai/gpt-5-mini", "sk-test")
	resp, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "classify"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if metadataHeader != "enabled" {
		t.Fatalf("X-OpenRouter-Metadata = %q, want enabled", metadataHeader)
	}
	got := resp.Attribution
	if got.RequestedProvider != "openrouter" || got.RequestedModel != "openai/gpt-5-mini" {
		t.Errorf("requested route = %q/%q", got.RequestedProvider, got.RequestedModel)
	}
	if got.ResolvedProvider != "OpenAI" || got.ResolvedModel != "openai/gpt-5-mini-2026-08-07" {
		t.Errorf("resolved route = %q/%q", got.ResolvedProvider, got.ResolvedModel)
	}
	if got.Attempts != 2 || got.GenerationID != "gen-123" {
		t.Errorf("attempts/id = %d/%q", got.Attempts, got.GenerationID)
	}
	if got.Tokens.Prompt != 194 || got.Tokens.Completion != 12 || got.Tokens.Reasoning != 5 ||
		got.Tokens.Cached != 31 || got.Tokens.CacheWrite != 7 || got.Tokens.Audio != 6 {
		t.Errorf("tokens = %+v", got.Tokens)
	}
	if got.Charge == nil || got.Charge.Amount != "0.0000012300" || got.Charge.Currency != "USD" {
		t.Errorf("charge = %+v", got.Charge)
	}
	if got.Latency <= 0 {
		t.Errorf("latency = %s, want positive", got.Latency)
	}
}

func TestOpenRouterAttributionKeepsMissingResolutionAndAttemptsUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gen-without-route","choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()

	provider, err := llm.NewOpenRouterChat(llm.OpenRouterChatConfig{
		BaseURL: srv.URL, Model: "openai/gpt-5-mini-2026-08-07", UpstreamProvider: "OpenAI",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := response.Attribution
	if got.RequestedProvider != "openrouter" || got.RequestedModel != "openai/gpt-5-mini-2026-08-07" {
		t.Fatalf("requested route = %+v", got)
	}
	if got.ResolvedProvider != "" || got.ResolvedModel != "" || got.Attempts != 0 {
		t.Fatalf("missing wire route was inferred: %+v", got)
	}
}

// A syntactically valid but wrong API base commonly returns the provider's HTML marketing page
// with status 200. The raw JSON decoder error ("invalid character '<'") sends the operator to the
// model output instead of the setting that is wrong, so name the response type and the URL field.
func TestOpenAI_HTMLResponsePointsAtTheAPIBaseSetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><title>Provider home page</title></html>"))
	}))
	defer srv.Close()

	_, err := llm.NewOpenAI(srv.URL, "m", "").Chat(
		context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{})
	if err == nil {
		t.Fatal("HTML response succeeded, want an actionable configuration error")
	}
	got := err.Error()
	for _, want := range []string{"expected JSON", "text/html", "llm.url", "API base"} {
		if !strings.Contains(got, want) {
			t.Errorf("error %q does not contain %q", got, want)
		}
	}
}

// ParseProvider accepts ollama|openai and rejects unknowns.
func TestParseProvider(t *testing.T) {
	for _, ok := range []string{"ollama", "openai"} {
		if _, err := llm.ParseProvider(ok); err != nil {
			t.Errorf("ParseProvider(%q) errored: %v", ok, err)
		}
	}
	if _, err := llm.ParseProvider("anthropic"); err == nil {
		t.Error("ParseProvider(anthropic) should error (unknown now)")
	}
}
