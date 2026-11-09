package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestModelPull_IsOwnedByApplicationGenerationNotRequest(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"status\":\"pulling manifest\"}\n"))
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer ollama.Close()

	st := testkit.MigratedSQLiteStore(t)
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	sut := &systemLLMService{
		swap:       llm.NewSwappable(func(llm.Selection) llm.Provider { return nil }, llm.Selection{Provider: "ollama"}),
		ollamaBase: func() string { return ollama.URL }, log: slog.Default(),
		newID: func() string { return "pull-owned" }, start: lifecycle.startInteractiveOperation,
		operations: st, pullTimeout: time.Hour,
	}
	requestCtx, disconnect := context.WithCancel(t.Context())
	if _, err := sut.Pull(requestCtx, "qwen3:8b"); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	<-started
	disconnect()
	select {
	case <-cancelled:
		t.Fatal("request disconnect cancelled accepted model pull")
	default:
	}

	shutdown := make(chan error, 1)
	go func() { shutdown <- application.Shutdown(t.Context()) }()
	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned before model pull stopped: %v", err)
	case <-cancelled:
	}
	if err := <-shutdown; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	operation, err := st.GetInteractiveOperation(t.Context(), "pull-owned")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Kind != store.InteractiveOperationLLMPull || operation.Status != store.InteractiveOperationError || operation.Error == "" {
		t.Fatalf("model pull operation = %+v, want durable cancellation", operation)
	}
}

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
	sut := &systemLLMService{
		swap:       llm.NewSwappable(func(llm.Selection) llm.Provider { return nil }, llm.Selection{Provider: "ollama"}),
		ollamaBase: func() string { return "http://ollama.invalid" },
		log:        slog.New(slog.DiscardHandler), newID: func() string { return "pull-timeout" },
		start: start, operations: st, pullTimeout: wantTimeout,
	}

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

// Selecting a model must HOT-APPLY, not merely land in the settings table
// (config-design §3, which names the §8.1 selection as its example of hot-apply).
//
// The regression, caught by the maintainer smoke on a genuinely fresh install: persist
// wrote through store.SetSetting, bypassing the settings service, so SetDB never ran
// and the in-memory snapshot kept the old value. The picker reported "In use" and the
// suggester really did swap (selectLocal calls swap.Set itself), but every settings
// *read* — including the wizard's `llm` check — still saw the previous value until the
// process restarted. So the wizard said "select a model in AI settings" to an operator
// who had just selected one.
//
// This asserts through Resolve (what every reader actually calls), NOT through the
// store row — reading the row back would have passed against the broken code.
func TestPersistSelection_HotAppliesToSettingsReaders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)

	reg := settings.NewRegistry()
	loader := settings.StoreLoader{List: func(ctx context.Context) ([]settings.SettingRow, error) {
		rows, err := st.ListSettings(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]settings.SettingRow, len(rows))
		for i, r := range rows {
			out[i] = settings.SettingRow{Key: r.Key, Value: r.Value, UpdatedBy: r.UpdatedBy}
		}
		return out, nil
	}}
	svc, err := settings.New(ctx, reg, loader, nil)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}

	// Wired exactly as buildLLM wires it — the seam under test is which write path
	// persist takes, so faking it here would test nothing.
	sut := &systemLLMService{
		saveSettings: func(ctx context.Context, edits map[string]string) error {
			results, err := svc.Patch(ctx, storePersister{st: st}, edits, "system")
			if err != nil {
				return err
			}
			for _, r := range results {
				if r.Status != settings.PatchSaved {
					t.Fatalf("Patch refused %s: %s", r.Key, r.Problem)
				}
			}
			return nil
		},
		store: st,
	}

	sel := llm.Selection{Provider: "ollama", URL: "http://localhost:11434", Model: "qwen3.5:9b"}
	if err := sut.persist(ctx, sel); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if got := svc.Resolve("llm.model").Value; got != sel.Model {
		t.Errorf("llm.model reads back %q, want %q — the write did not hot-apply, so the "+
			"wizard's llm check stays red until a restart", got, sel.Model)
	}
	if got := svc.Resolve("llm.provider").Value; got != sel.Provider {
		t.Errorf("llm.provider reads back %q, want %q", got, sel.Provider)
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

	sut := &systemLLMService{log: slog.Default()}
	err := sut.selectHosted(ctx, "openrouter", "", "qwen3:8b", "unused")
	if err == nil || !strings.Contains(err.Error(), "hosted model") {
		t.Fatalf("selectHosted(qwen3:8b) error = %v, want a hosted-model shape error", err)
	}
}

func TestSelectHosted_RejectsOpenRouterBatchModelBeforeCallingTheProvider(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // any provider call would fail differently; validation must happen first

	sut := &systemLLMService{log: slog.Default()}
	err := sut.selectHosted(ctx, "openrouter", "", "openai/gpt-4.1-nano:batch", "unused")
	if err == nil || !strings.Contains(err.Error(), "hosted model") {
		t.Fatalf("selectHosted(batch model) error = %v, want a hosted-model shape error", err)
	}
}
