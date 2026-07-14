package llm

import (
	"context"
	"testing"
)

// stubProvider records the model it was built for and echoes it back as content, so
// a test can see which underlying provider a Chat call routed to.
type stubProvider struct{ model string }

func (s stubProvider) Chat(_ context.Context, _ []Message, _ ChatOptions) (Response, error) {
	return Response{Content: s.model}, nil
}
func (s stubProvider) Name() string { return "stub" }

func TestSwappable_HotSwapsModel(t *testing.T) {
	built := 0
	sw := NewSwappable(func(model string) Provider {
		built++
		return stubProvider{model: model}
	}, "qwen3:8b")

	if got := sw.Model(); got != "qwen3:8b" {
		t.Fatalf("initial model = %q, want qwen3:8b", got)
	}
	// A Chat call routes to the initial model.
	resp, _ := sw.Chat(context.Background(), nil, ChatOptions{})
	if resp.Content != "qwen3:8b" {
		t.Errorf("chat routed to %q, want qwen3:8b", resp.Content)
	}

	// Swap, then the SAME provider value (as the suggester would hold) routes to the
	// new model — no rebuild of the suggester needed.
	sw.SetModel("llama3.1:8b")
	if got := sw.Model(); got != "llama3.1:8b" {
		t.Fatalf("after swap model = %q, want llama3.1:8b", got)
	}
	resp, _ = sw.Chat(context.Background(), nil, ChatOptions{})
	if resp.Content != "llama3.1:8b" {
		t.Errorf("chat after swap routed to %q, want llama3.1:8b", resp.Content)
	}
	if built != 2 {
		t.Errorf("build called %d times, want 2 (initial + swap)", built)
	}
}

// The wrapper satisfies Provider so the suggester can hold it transparently.
func TestSwappable_IsProvider(t *testing.T) {
	var _ Provider = NewSwappable(func(string) Provider { return stubProvider{} }, "m")
}
