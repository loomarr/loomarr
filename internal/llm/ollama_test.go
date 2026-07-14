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
