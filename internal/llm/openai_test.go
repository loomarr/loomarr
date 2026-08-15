package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/llm"
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
