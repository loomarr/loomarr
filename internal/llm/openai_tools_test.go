package llm_test

import (
	"context"
	"encoding/json"
	"github.com/loomarr/loomarr/internal/llm"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestOpenAIEnvelopesRootCombinatorsWithoutWeakeningSchema(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "oneOf": []any{map[string]any{"required": []any{"query"}}}, "additionalProperties": false}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var sent map[string]any
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			definitions, _ := sent["tools"].([]any)
			parameters := definitions[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
			properties, _ := parameters["properties"].(map[string]any)
			if !reflect.DeepEqual(properties["input"], schema) || parameters["oneOf"] != nil || parameters["additionalProperties"] != false {
				t.Errorf("complete schema must be nested under strict input envelope: %#v", parameters)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"call1","function":{"name":"catalog_search","arguments":"{\"input\":{\"query\":\"Matrix\"}}"}}]}}]}`))
		} else {
			messages := sent["messages"].([]any)
			for _, raw := range messages {
				m := raw.(map[string]any)
				for _, rawCall := range anyArray(m["tool_calls"]) {
					fn := rawCall.(map[string]any)["function"].(map[string]any)
					var args map[string]any
					if err := json.Unmarshal([]byte(fn["arguments"].(string)), &args); err != nil {
						t.Fatal(err)
					}
					if _, ok := args["input"]; !ok {
						t.Errorf("finalization history lost wire envelope: %v", args)
					}
				}
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
		}
	}))
	defer srv.Close()
	provider := llm.NewOpenAI(srv.URL, "synthetic-model", "")
	response, err := provider.Chat(context.Background(), nil, llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search", Parameters: schema}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Arguments["query"] != "Matrix" {
		t.Fatalf("normalized arguments=%+v", response)
	}
	history := []llm.Message{{Role: llm.Assistant, ToolCalls: response.ToolCalls}, {Role: llm.Tool, ToolCallID: "call1", Content: "[]"}, {Role: llm.Assistant, ToolCalls: []llm.ToolCall{{ID: "source1", Name: "catalog_search", Arguments: map[string]any{"query": "Source"}}}}}
	if _, err := provider.Chat(context.Background(), history, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["oneOf"]; !ok {
		t.Fatal("caller schema mutated")
	}
}

func anyArray(value any) []any { values, _ := value.([]any); return values }

func TestOpenAIToolEnvelopeRejectsMalformedShapes(t *testing.T) {
	for _, raw := range []string{`{"query":"bypass"}`, `{"input":[],"query":"bypass"}`, `{"input":{"query":"valid"},"query":"bypass"}`} {
		t.Run(raw, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "call", "function": map[string]any{"name": "catalog_search", "arguments": raw}}}}}}})
			}))
			defer server.Close()
			p := llm.NewOpenAI(server.URL, "synthetic", "")
			response, err := p.Chat(context.Background(), nil, llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search", Parameters: map[string]any{"type": "object", "allOf": []any{map[string]any{"required": []string{"query"}}}}}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(response.ToolCalls) != 1 || len(response.ToolCalls[0].Arguments) != 0 {
				t.Fatalf("malformed envelope exposed usable arguments: %+v", response)
			}
		})
	}
}

func TestOpenAIRejectsRelocatingSchemaReferencesBeforeInference(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	p := llm.NewOpenAI(server.URL, "synthetic", "")
	_, err := p.Chat(context.Background(), nil, llm.ChatOptions{Tools: []llm.ToolSchema{{Name: "catalog_search", Parameters: map[string]any{"type": "object", "allOf": []map[string]any{{"$ref": "#/$defs/request"}}}}}})
	if err == nil || calls != 0 {
		t.Fatalf("references must fail before inference: calls=%d err=%v", calls, err)
	}
}
