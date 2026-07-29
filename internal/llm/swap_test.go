package llm

import (
	"context"
	"testing"
)

// stubProvider records the model it was built for and echoes it back as content, so
// a test can see which underlying provider a Chat call routed to.
type stubProvider struct {
	model string
	name  string
}

func (s stubProvider) Chat(_ context.Context, _ []Message, _ ChatOptions) (Response, error) {
	return Response{Content: s.model}, nil
}
func (s stubProvider) Name() string {
	if s.name != "" {
		return s.name
	}
	return "stub"
}

func TestSwappable_HotSwapsModel(t *testing.T) {
	built := 0
	sw := NewSwappable(func(sel Selection) Provider {
		built++
		return stubProvider{model: sel.Model, name: sel.Provider}
	}, Selection{Provider: "ollama", Model: "qwen3:8b"})

	if got := sw.Model(); got != "qwen3:8b" {
		t.Fatalf("initial model = %q, want qwen3:8b", got)
	}
	resp, _ := sw.Chat(context.Background(), nil, ChatOptions{})
	if resp.Content != "qwen3:8b" {
		t.Errorf("chat routed to %q, want qwen3:8b", resp.Content)
	}

	// SetModel keeps the provider, swaps the tag — the same provider value the
	// suggester holds now routes to the new model, no rebuild of the suggester.
	sw.SetModel("llama3.1:8b")
	if got := sw.Model(); got != "llama3.1:8b" {
		t.Fatalf("after swap model = %q, want llama3.1:8b", got)
	}
	if got := sw.Provider(); got != "ollama" {
		t.Errorf("SetModel changed provider to %q, want it kept as ollama", got)
	}
	resp, _ = sw.Chat(context.Background(), nil, ChatOptions{})
	if resp.Content != "llama3.1:8b" {
		t.Errorf("chat after swap routed to %q, want llama3.1:8b", resp.Content)
	}
	if built != 2 {
		t.Errorf("build called %d times, want 2 (initial + swap)", built)
	}
}

// A full-provider swap (local → hosted) changes provider, URL, model, and key
// together — the hosted-selection case.
func TestSwappable_FullProviderSwap(t *testing.T) {
	sw := NewSwappable(func(sel Selection) Provider {
		return stubProvider{model: sel.Model, name: sel.Provider}
	}, Selection{Provider: "ollama", URL: "http://ollama:11434", Model: "qwen3:8b"})

	if sw.Name() != "ollama" {
		t.Fatalf("initial Name() = %q, want ollama", sw.Name())
	}
	sw.Set(Selection{Provider: "openai", URL: "https://openrouter.ai/api/v1", Model: "openai/gpt-4o-mini", APIKey: "sk-x"})
	if sw.Provider() != "openai" || sw.Model() != "openai/gpt-4o-mini" {
		t.Errorf("after swap = %q/%q, want openai/openai/gpt-4o-mini", sw.Provider(), sw.Model())
	}
	if sw.Name() != "openai" {
		t.Errorf("Name() after provider swap = %q, want openai", sw.Name())
	}
	// The key rides in the selection but is never exposed via Chat routing here; the
	// factory closure is what consumes it (asserted via provider name change).
	if sw.Selection().APIKey != "sk-x" {
		t.Error("selection lost the api key on swap")
	}
}

func TestSwappable_IsProvider(t *testing.T) {
	var _ Provider = NewSwappable(func(Selection) Provider { return stubProvider{} }, Selection{})
}

// warmSpy is a Provider that records Warm calls (§8.2).
type warmSpy struct {
	Provider
	warmed int
}

func (w *warmSpy) Warm(context.Context) error { w.warmed++; return nil }

// A Swappable forwards Warm to the ACTIVE provider when it supports preloading.
func TestSwappable_WarmForwardsToActiveProvider(t *testing.T) {
	spy := &warmSpy{Provider: &stubProvider{name: "ollama"}}
	sw := NewSwappable(func(Selection) Provider { return spy }, Selection{Provider: "ollama"})

	if err := sw.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if spy.warmed != 1 {
		t.Errorf("Warm calls = %d, want 1", spy.warmed)
	}
}

// A provider that can't preload (any hosted endpoint — there is no residency to
// manage) is a silent no-op, NOT an error. Returning one would make every caller
// log a failure for the ordinary hosted case, training operators to ignore it.
func TestSwappable_WarmIsNoOpForANonWarmerProvider(t *testing.T) {
	sw := NewSwappable(
		func(Selection) Provider { return &stubProvider{name: "openai"} },
		Selection{Provider: "openai"},
	)
	if err := sw.Warm(context.Background()); err != nil {
		t.Errorf("Warm on a non-Warmer provider should be a no-op, got %v", err)
	}
}

// KeepAlive is part of the Selection, so a swap must CARRY it — a rebuild that
// dropped it would silently revert to the 5-minute idle unload and quietly bring
// the ~9s cold load back on the next run.
func TestSwappable_SwapCarriesKeepAlive(t *testing.T) {
	var got []string
	sw := NewSwappable(func(sel Selection) Provider {
		got = append(got, sel.KeepAlive)
		return &stubProvider{name: "ollama"}
	}, Selection{Provider: "ollama", Model: "a", KeepAlive: "30m"})

	sw.SetModel("b") // the common local case: pick a different tag on the same host

	if len(got) != 2 {
		t.Fatalf("expected 2 builds (initial + swap), got %d", len(got))
	}
	if got[1] != "30m" {
		t.Errorf("keep_alive after a model swap = %q, want %q", got[1], "30m")
	}
}
