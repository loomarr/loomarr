package picker

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestModelPull_HasExplicitTimeoutAndDurableTerminalOutcome(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	wantTimeout := 37 * time.Minute
	start := func(
		timeout time.Duration,
		run func(context.Context) error,
		complete func(context.Context, error),
	) error {
		if timeout != wantTimeout {
			t.Fatalf("model pull timeout = %v, want %v", timeout, wantTimeout)
		}
		operationCtx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		complete(t.Context(), run(operationCtx))
		return nil
	}
	sut := New(Config{
		Swap:       llm.NewSwappable(func(llm.Selection) llm.Provider { return nil }, llm.Selection{Provider: "ollama"}),
		OllamaBase: func() string { return "http://ollama.invalid" },
		Log:        slog.New(slog.DiscardHandler), NewID: func() string { return "pull-timeout" },
		Start: start, Operations: st, PullTimeout: wantTimeout,
	})

	jobID, err := sut.Pull(t.Context(), "qwen3:8b")
	if err != nil || jobID != "pull-timeout" {
		t.Fatalf("Pull = %q, %v", jobID, err)
	}
	operation, err := st.GetInteractiveOperation(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != store.InteractiveOperationError || !strings.Contains(operation.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("timed out pull = %+v, want durable deadline error", operation)
	}
}

// wireKind maps a (possibly branded) provider to the llm.provider enum. Every hosted
// provider is an OpenAI-compatible client, so it must persist as "openai" — the fix
// for the 502 "openrouter is not one of [ollama openai]" that broke the hosted picker.
func TestWireKind(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":           "ollama", // back-compat default
		"ollama":     "ollama",
		"openai":     "openai",
		"openrouter": "openai", // branded → wire kind
		"custom":     "openai",
	}
	for in, want := range cases {
		if got := wireKind(in); got != want {
			t.Errorf("wireKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectHosted_RejectsAnOllamaTagBeforeCallingTheProvider(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // any provider call would fail differently; validation must happen first

	sut := New(Config{Log: slog.Default()})
	err := sut.selectHosted(ctx, "openrouter", "", "qwen3:8b", "unused")
	if err == nil || !strings.Contains(err.Error(), "hosted model") {
		t.Fatalf("selectHosted(qwen3:8b) error = %v, want a hosted-model shape error", err)
	}
}

func TestSelectHosted_RejectsOpenRouterBatchModelBeforeCallingTheProvider(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // any provider call would fail differently; validation must happen first

	sut := New(Config{Log: slog.Default()})
	err := sut.selectHosted(ctx, "openrouter", "", "openai/gpt-4.1-nano:batch", "unused")
	if err == nil || !strings.Contains(err.Error(), "hosted model") {
		t.Fatalf("selectHosted(batch model) error = %v, want a hosted-model shape error", err)
	}
}
