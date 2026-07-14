package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/testkit"
)

// Against the pinned Ollama capture: a request carrying a tool yields a response
// whose tool_calls[].function.arguments is a PARSED OBJECT (FINDINGS finding 1),
// which the provider surfaces as ToolCall.Arguments (a map).
func TestOllama_ToolCallArgumentsAreObject(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_toolcall_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "llama3.1:8b")
	got, err := o.Chat(context.Background(), []llm.Message{
		{Role: llm.System, Content: "no inventing"},
		{Role: llm.User, Content: "build a channel"},
	}, llm.ChatOptions{
		Tools: []llm.ToolSchema{{
			Name: "catalog_search", Description: "search",
			Parameters: map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.WantsTools() {
		t.Fatal("expected the model to request a tool call")
	}
	tc := got.ToolCalls[0]
	if tc.Name != "catalog_search" {
		t.Errorf("tool name = %q", tc.Name)
	}
	// Arguments came through as a decoded object, not a string.
	if tc.Arguments["query"] != "90s action movie" {
		t.Errorf("tool arguments not decoded as object: %+v", tc.Arguments)
	}
	// The request carried stream:false and the tool.
	if sentReq["stream"] != false {
		t.Errorf("request stream = %v, want false", sentReq["stream"])
	}
	if _, ok := sentReq["tools"]; !ok {
		t.Error("request missing tools array")
	}
}

// Against the pinned final response: after a tool result, the model returns
// grounded JSON content and no tool calls.
func TestOllama_FinalContentAndJSONMode(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "")
	got, err := o.Chat(context.Background(), []llm.Message{
		{Role: llm.Tool, Content: `[{"tmdb_id":100,"title":"Speed"}]`, ToolCallID: "call_1"},
	}, llm.ChatOptions{JSONMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.WantsTools() {
		t.Error("final turn should have no tool calls")
	}
	if got.Content == "" {
		t.Fatal("expected final JSON content")
	}
	// JSONMode sets format:json on the request.
	if sentReq["format"] != "json" {
		t.Errorf("JSONMode should set format=json, got %v", sentReq["format"])
	}
	// The content is valid JSON with the grounded ids.
	var parsed struct {
		Picks []struct {
			TMDBID int `json:"tmdb_id"`
		} `json:"picks"`
	}
	if err := json.Unmarshal([]byte(got.Content), &parsed); err != nil {
		t.Fatalf("final content not valid JSON: %v", err)
	}
	if len(parsed.Picks) == 0 || parsed.Picks[0].TMDBID != 100 {
		t.Errorf("final content lost the grounded id: %+v", parsed.Picks)
	}
}

// T0.1: the Ollama request carries an `options` block with num_ctx always set (so
// the growing tool loop isn't truncated) and the forwarded sampling controls.
func TestOllama_SendsOptionsBlock(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	o := llm.NewOllama(srv.URL, "")
	temp := 0.2
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}},
		llm.ChatOptions{Temperature: &temp, MaxTokens: 512}); err != nil {
		t.Fatal(err)
	}
	opts, ok := sentReq["options"].(map[string]any)
	if !ok {
		t.Fatalf("request has no options block: %v", sentReq["options"])
	}
	if opts["num_ctx"] == nil {
		t.Error("num_ctx must always be set (tool loop must not be truncated)")
	}
	if opts["temperature"] != 0.2 {
		t.Errorf("temperature = %v, want 0.2 forwarded", opts["temperature"])
	}
	if opts["num_predict"] != float64(512) {
		t.Errorf("num_predict = %v, want 512 (from MaxTokens)", opts["num_predict"])
	}
}

// Thinking is disabled when tools are present: a reasoning model's chain-of-thought
// breaks tool-calls/JSON on Ollama (bugs #10976/#14601). Asserts think:false is sent
// with tools, and NOT sent (field omitted) without them.
func TestOllama_DisablesThinkingWithTools(t *testing.T) {
	resp := testkit.Fixture(t, "llm/ollama_final_response.json")
	var sentReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sentReq)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()
	o := llm.NewOllama(srv.URL, "")

	// With tools → think:false.
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}},
		llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search"}}}); err != nil {
		t.Fatal(err)
	}
	if think, ok := sentReq["think"]; !ok || think != false {
		t.Errorf("with tools, think should be false, got %v (present=%v)", think, ok)
	}

	// Without tools → think omitted (don't touch a non-thinking model's default).
	sentReq = nil
	if _, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "x"}}, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sentReq["think"]; ok {
		t.Error("without tools, think should be omitted")
	}
}
