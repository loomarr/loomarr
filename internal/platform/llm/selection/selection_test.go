package selection

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
)

func TestHostedSelectionMatchesGenericOpenAIByCanonicalURL(t *testing.T) {
	t.Parallel()
	openRouter, _ := llm.HostedProviderByKey("openrouter")
	custom, _ := llm.HostedProviderByKey(llm.CustomProviderKey)

	if !HostedSelectionMatches(llm.Selection{
		Provider: "openai", URL: openRouter.BaseURL + "/",
	}, openRouter) {
		t.Error("generic openai selection at the canonical OpenRouter URL should activate OpenRouter")
	}
	if !HostedSelectionMatches(llm.Selection{
		Provider: "openai", URL: "http://ai.internal/v1",
	}, custom) {
		t.Error("generic openai selection at an operator URL should activate Custom")
	}
}

// KeepAliveArg renders the residency duration for Ollama's wire format (§8.2). The two
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
			if got := KeepAliveArg(tc.in); got != tc.want {
				t.Errorf("KeepAliveArg(%v) = %q, want %q", tc.in, got, tc.want)
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

// syncBuf is a mutex-guarded log sink. Warm logs from a goroutine it owns, so the
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

			Warm(context.Background(), tc.warmer, log)

			// Warm is fire-and-forget, so poll for the line. The silent case has nothing
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
