package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/testkit"
)

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

func TestHostedSelectionMatchesGenericOpenAIByCanonicalURL(t *testing.T) {
	t.Parallel()
	openRouter, _ := llm.HostedProviderByKey("openrouter")
	custom, _ := llm.HostedProviderByKey(llm.CustomProviderKey)

	if !hostedSelectionMatches(llm.Selection{
		Provider: "openai", URL: openRouter.BaseURL + "/",
	}, openRouter) {
		t.Error("generic openai selection at the canonical OpenRouter URL should activate OpenRouter")
	}
	if !hostedSelectionMatches(llm.Selection{
		Provider: "openai", URL: "http://ai.internal/v1",
	}, custom) {
		t.Error("generic openai selection at an operator URL should activate Custom")
	}
}

// keepAliveArg renders the residency duration for Ollama's wire format (§8.2). The two
// edge values are the point: 0 means "unload as soon as this request finishes" and a
// negative means "keep loaded indefinitely" — both are real instructions to Ollama, so
// neither may collapse to "" (which would instead mean "inherit Ollama's 5m default",
// silently reversing what the operator asked for).
func TestKeepAliveArg(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   time.Duration
		want string
	}{
		{"a normal residency", 30 * time.Minute, "30m0s"},
		{"zero unloads immediately, and must not read as unset", 0, "0"},
		{"negative keeps the model loaded indefinitely", -1, "-1ns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := keepAliveArg(tc.in); got != tc.want {
				t.Errorf("keepAliveArg(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// nothingWarmer declines to warm, the fresh-install case (no model picked yet).
type nothingWarmer struct{}

func (nothingWarmer) Warm(context.Context) error { return llm.ErrNothingToWarm }

// failWarmer reports a real warm-up failure (e.g. Ollama not up yet).
type failWarmer struct{}

func (failWarmer) Warm(context.Context) error { return errors.New("connection refused") }

// okWarmer warms successfully.
type okWarmer struct{}

func (okWarmer) Warm(context.Context) error { return nil }

// syncBuf is a mutex-guarded log sink. warm() logs from a goroutine it owns, so the
// test reads what that goroutine wrote — an unguarded bytes.Buffer is a real data race
// here, and `go test -race` says so.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The three warm-up outcomes must be distinguishable in the log, because two of them
// would otherwise be a lie (§8.2). A fresh install with no model chosen made this
// concrete twice: it first logged `warm-up skipped err="status 404"` (a failure report
// for a model nobody picked), and then, once that request was suppressed but the
// outcome collapsed to nil, `llm model warmed took=0` — announcing a preload that
// never happened. "Nothing to warm" is a third outcome and says nothing at all.
func TestWarm_LogsEachOutcomeHonestly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		warmer  llm.Warmer
		wantLog string // "" = must log nothing
	}{
		{"nothing to warm is silent", nothingWarmer{}, ""},
		{"a real failure is reported", failWarmer{}, "warm-up skipped"},
		{"a real warm-up is reported", okWarmer{}, "model warmed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf syncBuf
			log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			warm(context.Background(), tc.warmer, log)

			// warm() is fire-and-forget, so poll for the line. The silent case has nothing
			// to wait for, so it polls out — that is the assertion, not a timeout.
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if buf.String() != "" {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}

			got := buf.String()
			if tc.wantLog == "" {
				if got != "" {
					t.Errorf("expected silence for a declined warm-up, logged: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantLog) {
				t.Errorf("log = %q, want it to contain %q", got, tc.wantLog)
			}
		})
	}
}
