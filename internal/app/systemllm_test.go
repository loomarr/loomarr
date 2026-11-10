package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/llm/picker"
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
	sut := &systemLLMService{picker: picker.New(picker.Config{
		Swap:       llm.NewSwappable(func(llm.Selection) llm.Provider { return nil }, llm.Selection{Provider: "ollama"}),
		OllamaBase: func() string { return ollama.URL }, Log: slog.Default(),
		NewID: func() string { return "pull-owned" }, Start: lifecycle.startInteractiveOperation,
		Operations: st, PullTimeout: time.Hour,
	})}
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
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Shutdown did not return while a model pull was running")
	}
	// The server sees the client's cancellation asynchronously, so it may follow Shutdown's return.
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("generation shutdown never cancelled the accepted model pull")
	}
	operation, err := st.GetInteractiveOperation(t.Context(), "pull-owned")
	if err != nil {
		t.Fatal(err)
	}
	if operation.Kind != store.InteractiveOperationLLMPull || operation.Status != store.InteractiveOperationError || operation.Error == "" {
		t.Fatalf("model pull operation = %+v, want durable cancellation", operation)
	}
}

// Selecting a model must HOT-APPLY, not merely land in the settings table
// (config-design §3, which names the §8.1 selection as its example of hot-apply).
//
// The regression, caught by the maintainer smoke on a genuinely fresh install: the
// selection was written through store.SetSetting, bypassing the settings service, so SetDB
// never ran and the in-memory snapshot kept the old value. The picker reported "In use" and
// the suggester really did swap, but every settings *read* — including the wizard's `llm`
// check — still saw the previous value until the process restarted. So the wizard said
// "select a model in AI settings" to an operator who had just selected one.
//
// This drives Select through the saver buildLLM wires and asserts through Resolve (what every
// reader actually calls), NOT through the store row — reading the row back would have passed
// against the broken code.
func TestSelect_HotAppliesToSettingsReaders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	ollama := testkit.NewOllama(t)
	ollama.SetPulled("qwen3.5:9b")

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

	// Wired exactly as buildLLM wires it — the seam under test is which write path the
	// selection takes, so faking it here would test nothing.
	sut := &systemLLMService{picker: picker.New(picker.Config{
		Swap:         llm.NewSwappable(func(llm.Selection) llm.Provider { return nil }, llm.Selection{Provider: "ollama"}),
		OllamaBase:   func() string { return ollama.URL },
		SaveSettings: llmSettingsSaver(resolved{svc: svc}, st, nil),
		Log:          slog.New(slog.DiscardHandler),
	})}

	const model = "qwen3.5:9b"
	if err := sut.Select(ctx, system.SelectRequest{Model: model}); err != nil {
		t.Fatalf("Select: %v", err)
	}

	if got := svc.Resolve("llm.model").Value; got != model {
		t.Errorf("llm.model reads back %q, want %q — the write did not hot-apply, so the "+
			"wizard's llm check stays red until a restart", got, model)
	}
	if got := svc.Resolve("llm.provider").Value; got != "ollama" {
		t.Errorf("llm.provider reads back %q, want %q", got, "ollama")
	}
}
