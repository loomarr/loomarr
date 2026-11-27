package picker

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/operationstore"
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
	if operation.Status != operationstore.InteractiveOperationError || !strings.Contains(operation.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("timed out pull = %+v, want durable deadline error", operation)
	}
}

// pullFrames records each pull frame with the operation's stored status at that moment.
type pullFrames struct {
	t      *testing.T
	st     store.Store
	frames []string
}

func (p *pullFrames) PullProgress(jobID, _, status string, _ int, _, _ int64, _ string) {
	operation, err := p.st.GetInteractiveOperation(p.t.Context(), jobID)
	if err != nil {
		p.t.Fatal(err)
	}
	p.frames = append(p.frames, status+"/"+string(operation.Status))
}

// Ollama ends a pull with its own "success" line before the operation's outcome is stored. Only
// the completion announces success, once, after storing it: a client that reads the operation on
// a frame sees what the frame says.
func TestModelPull_AnnouncesSuccessOnceItIsStored(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"status\":\"pulling manifest\"}\n{\"status\":\"success\"}\n"))
	}))
	defer ollama.Close()
	st := testkit.MigratedSQLiteStore(t)
	frames := &pullFrames{t: t, st: st}
	start := func(_ time.Duration, run func(context.Context) error, complete func(context.Context, error)) error {
		complete(t.Context(), run(t.Context()))
		return nil
	}
	sut := New(Config{
		Swap:       llm.NewSwappable(func(llm.Selection) llm.Provider { return nil }, llm.Selection{Provider: "ollama"}),
		OllamaBase: func() string { return ollama.URL },
		Log:        slog.New(slog.DiscardHandler), NewID: func() string { return "pull-1" },
		Start: start, Operations: st, PullTimeout: time.Minute, Events: frames,
	})

	if _, err := sut.Pull(t.Context(), "qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	want := []string{"starting/running", "pulling manifest/running", "success/success"}
	if !slices.Equal(frames.frames, want) {
		t.Fatalf("frames = %v, want %v", frames.frames, want)
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
