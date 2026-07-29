package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/store"
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
func TestBuildHandler_RepeatsWithoutLeaking(t *testing.T) {
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

	h, err := BuildHandler(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatalf("generation %d: build handler: %v", generation, err)
	}

	// Actually serve. A handler that is built and never used would not exercise the
	// lazily-started machinery a real generation starts.
	srv := httptest.NewServer(h)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("generation %d: serve: %v", generation, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generation %d: /healthz → %d, want 200", generation, resp.StatusCode)
	}
	srv.Close()

	// Teardown in runOnce's order: cancel background work, then release the store.
	cancel()
	settle()
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
func TestBootstrapDrift(t *testing.T) {
	t.Run("no drift when the running config matches the environment", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "sqlite:///data/a.db")
		running, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := bootstrapDrift(running)(); len(got) != 0 {
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

		got := bootstrapDrift(running)()
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
		drift := bootstrapDrift(running)

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
		if got := bootstrapDrift(nil)(); got != nil {
			t.Errorf("drift = %v with no baseline, want nil", got)
		}
	})
}
