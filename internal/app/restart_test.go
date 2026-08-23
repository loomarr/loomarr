package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/config"
	"github.com/loomarr/loomarr/internal/store"
)

// ⚠ THE GATE FOR §9.2. Restarting in place is only correct if Build/Run/Shutdown can
// repeat without accumulating goroutines or inheriting the previous generation's closed
// resources — and both failures are SILENT. A leaked goroutine degrades an install over
// successive restarts; a package-level `sync.Once` hands generation 2 a closed handle with
// no panic and no log line. Neither shows up in a functional test of one boot.
//
// So the gate is repetition: build the real handler N times against a real store, serve a
// request from each generation, and assert the goroutine count is stable across the last
// few. A prose rule ("don't use package-level mutable state") would not have caught it.
func TestBuild_RepeatsWithoutLeaking(t *testing.T) {
	const generations = 5

	// Warm one generation before measuring. The first build legitimately starts
	// long-lived infrastructure (the metrics collector's registration, sql driver
	// internals, http transport pools) that later generations reuse — counting from
	// zero would measure that one-time cost as a leak.
	counts := make([]int, 0, generations)
	for i := 1; i <= generations; i++ {
		buildServeShutdown(t, i)
		// Goroutines exit asynchronously after their context is cancelled, so settle
		// before counting or the measurement races the teardown it is measuring.
		settle()
		counts = append(counts, runtime.NumGoroutine())
	}

	// Compare the LAST generation against the second — skipping the first, whose
	// one-time setup is not a leak. A leak is monotonic growth, so any real one shows
	// up across four iterations.
	first, last := counts[1], counts[len(counts)-1]
	if grew := last - first; grew > 2 {
		t.Errorf("goroutines grew by %d across %d generations (%v) — a restart leaks",
			grew, generations-1, counts)
	}
}

// buildServeShutdown is one generation: build the real handler, serve one request through
// it, then tear it down exactly as runOnce does.
func buildServeShutdown(t *testing.T, generation int) {
	t.Helper()

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/gen.db", true)
	if err != nil {
		t.Fatalf("generation %d: open store: %v", generation, err)
	}
	defer func() { _ = st.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatalf("generation %d: build application: %v", generation, err)
	}
	h := application.Handler()

	// Actually serve. A handler that is built and never used would not exercise the
	// lazily-started machinery a real generation starts.
	srv := httptest.NewServer(h)
	resp, err := http.Get(srv.URL + "/v1/healthz")
	if err != nil {
		t.Fatalf("generation %d: serve: %v", generation, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generation %d: /v1/healthz → %d, want 200", generation, resp.StatusCode)
	}

	metricsResp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("generation %d: scrape metrics: %v", generation, err)
	}
	body, readErr := io.ReadAll(metricsResp.Body)
	_ = metricsResp.Body.Close()
	if readErr != nil {
		t.Fatalf("generation %d: read metrics: %v", generation, readErr)
	}
	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("generation %d: /metrics → %d, want 200", generation, metricsResp.StatusCode)
	}
	for _, family := range []string{"loomarr_titles", "loomarr_jobs", "loomarr_active_sessions"} {
		if !strings.Contains(string(body), family) {
			t.Errorf("generation %d: /metrics missing %s; collector retained an old store", generation, family)
		}
	}
	srv.Close()

	// Teardown in runOnce's order: cancel background work, then release the store.
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("generation %d: shutdown application: %v", generation, err)
	}
}

// settle waits for cancelled goroutines to exit, stopping as soon as the count holds
// steady. Polling rather than a flat sleep so a fast machine is not penalised and a slow
// one is not flaky.
func settle() {
	prev := -1
	stable := 0
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n == prev {
			if stable++; stable >= 3 {
				return
			}
			continue
		}
		prev, stable = n, 0
	}
}

// The RestartRequired flag is DERIVED from running-vs-resolved bootstrap values, so it
// cannot nag about a restart the operator already undid (config-design §3).
func TestRestartDrift(t *testing.T) {
	t.Run("no drift when the running config matches the environment", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := restartDrift(running, nil, nil)(); len(got) != 0 {
			t.Errorf("drift = %v, want none — nothing changed", got)
		}
	})

	t.Run("names the specific key that changed", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		// The operator saves a new database target; this process is still on the old one.
		t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/loomarr")

		got := restartDrift(running, nil, nil)()
		if len(got) != 1 || got[0] != "DATABASE_URL" {
			t.Errorf("drift = %v, want [DATABASE_URL] — the UI must name WHICH setting", got)
		}
	})

	t.Run("clears when the change is reverted", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		drift := restartDrift(running, nil, nil)

		t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/loomarr")
		if len(drift()) == 0 {
			t.Fatal("expected drift after the change")
		}
		// ⚠ The whole reason this is derived rather than a sticky boolean: undoing the
		// edit must stop the nagging, and a flag set at save time never would.
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		if got := drift(); len(got) != 0 {
			t.Errorf("drift = %v after reverting, want none", got)
		}
	})

	t.Run("a nil baseline reports no drift", func(t *testing.T) {
		// Safe direction: a false "restart required" points the operator at an action
		// that cannot help.
		if got := restartDrift(nil, nil, nil)(); got != nil {
			t.Errorf("drift = %v with no baseline, want nil", got)
		}
	})

	t.Run("combines bootstrap and generation drift deterministically", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/loomarr")
		desired := map[string]string{
			"filler.dir":       "/clips/new",
			"filler.watch_dir": "/watch/new",
		}
		drift := restartDrift(running, map[string]string{
			"filler.watch_dir": "/watch/old",
			"filler.dir":       "/clips/old",
		}, func(key string) string { return desired[key] })

		got := drift()
		want := []string{"DATABASE_URL", "filler.dir", "filler.watch_dir"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("drift = %v, want %v", got, want)
		}
	})

	t.Run("generation drift clears when desired values are reverted", func(t *testing.T) {
		desired := map[string]string{"filler.dir": "/clips/new"}
		drift := restartDrift(nil, map[string]string{"filler.dir": "/clips/old"},
			func(key string) string { return desired[key] })
		if got := drift(); len(got) != 1 || got[0] != "filler.dir" {
			t.Fatalf("drift = %v, want [filler.dir]", got)
		}
		desired["filler.dir"] = "/clips/old"
		if got := drift(); len(got) != 0 {
			t.Errorf("drift after revert = %v, want none", got)
		}
	})
}
