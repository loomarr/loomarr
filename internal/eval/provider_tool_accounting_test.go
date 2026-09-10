//go:build eval

package eval

import (
	"context"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/testkit"
	"testing"
)

func TestObservedProviderAttributesOnlyAcknowledgedModelCalls(t *testing.T) {
	real := llm.ToolCall{ID: "model-call", Name: "catalog_search", Arguments: map[string]any{"network": "ABC"}}
	// A returned but unexecuted finalization call is not a dispatched operation.
	malformed := llm.ToolCall{ID: "model-call", Name: "catalog_search", Arguments: map[string]any{"cast": []any{"Actor"}}}
	provider := &observedProvider{inner: testkit.NewLLM(llm.Response{ToolCalls: []llm.ToolCall{real}}, llm.Response{ToolCalls: []llm.ToolCall{malformed}})}
	provider.Begin()
	synthetic := llm.Message{Role: llm.Assistant, ToolCalls: []llm.ToolCall{{ID: "model-call", Name: "catalog_search", Arguments: map[string]any{"query": "Source anchor"}}}}
	// Reusing a real model id in synthesized history cannot create another operation.
	history := []llm.Message{synthetic}
	if _, err := provider.Chat(context.Background(), history, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	history = append(history, llm.Message{Role: llm.Assistant, ToolCalls: []llm.ToolCall{real}}, synthetic)
	if _, err := provider.Chat(context.Background(), history, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	got := provider.Snapshot(nil)
	if got.ToolCalls != 1 || got.NetworkCalls != 1 || got.CastCalls != 0 || got.TitleCalls != 0 {
		t.Fatalf("dispatched model operations must exclude synthesized history and unexecuted final output: %+v", got)
	}
}
