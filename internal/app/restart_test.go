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

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/platform/config"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
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

	st, err := store.Open(context.Background(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatalf("generation %d: open store: %v", generation, err)
	}
	defer func() { _ = st.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{MetricsToken: "restart-test-scrape-token"})
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

	metricsReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/metrics", nil)
	metricsReq.Header.Set("Authorization", "Bearer restart-test-scrape-token")
	metricsResp, err := http.DefaultClient.Do(metricsReq)
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
	// Teardown in runOnce's two-phase order: cancel background work and quiesce network resources,
	// drain HTTP, finalize the generation, then release the store through the defer above.
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := application.Quiesce(shutdownCtx); err != nil {
		t.Fatalf("generation %d: quiesce application: %v", generation, err)
	}
	if err := srv.Config.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("generation %d: drain HTTP: %v", generation, err)
	}
	if err := application.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("generation %d: shutdown application: %v", generation, err)
	}
	srv.Close()
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

// #1659 Q-S1: the restart notice names only what truly cannot apply live. Built the way Build
// builds it, from the registry's restart keys, so a key that goes live drops out of the notice.
func TestRestartDrift_NamesOnlyTheFillerFoldersAmongAppSettings(t *testing.T) {
	set := visionSet(t, map[string]string{
		"filler.dir":       "/clips/old",
		"filler.watch_dir": "/watch/old",
	})
	frozen, applied := set.freeze(settings.NewRegistry().RestartKeys()...)
	layout, err := cliplibrary.NewLayout(settings.FillerDir.Get(frozen), settings.FillerWatchDir.Get(frozen))
	if err != nil {
		t.Fatal(err)
	}
	canonicalFillerRestartBaseline(applied, layout)
	drift := config.RestartDrift(nil, applied, canonicalRestartCurrent(set))

	set.svc.SetDB(map[string]string{
		"filler.dir":                              "/clips/old",
		"filler.watch_dir":                        "/watch/old",
		"diagnostics.dir":                         "/diagnostics/new",
		"filler.structure_window_authority_path":  "/reviewed/authority.json",
		"filler.structure_window_deployment_path": "/reviewed/deployment.json",
	})
	if got := drift(); len(got) != 0 {
		t.Fatalf("drift = %v after editing live settings, want none", got)
	}
	set.svc.SetDB(map[string]string{"filler.dir": "/clips/new", "filler.watch_dir": "/watch/new"})
	if got := strings.Join(drift(), ","); got != "filler.dir,filler.watch_dir" {
		t.Errorf("drift = %v, want the filler folders", got)
	}
}
