package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestFoundationStorageBudgetAndProjectionSurviveSQLiteRestart(t *testing.T) {
	testFoundationStorageBudgetAndProjectionSurviveRestart(t, testkit.MigratedSQLiteStore(t))
}

func testFoundationStorageBudgetAndProjectionSurviveRestart(t *testing.T, st store.Store) {
	t.Helper()
	root := t.TempDir()
	fillerDir := filepath.Join(root, "filler")
	watchDir := filepath.Join(fillerDir, "_watch")
	preparedDir := filepath.Join(root, "prepared")
	diagnosticsDir := filepath.Join(root, "diagnostics")
	for _, dir := range []string{fillerDir, watchDir, preparedDir, diagnosticsDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FILLER_DIR", fillerDir)
	t.Setenv("FILLER_WATCH_DIR", watchDir)
	t.Setenv("PLAYOUT_PREPARED_DIR", preparedDir)
	t.Setenv("DIAGNOSTICS_DIR", diagnosticsDir)
	if err := st.SetSetting(t.Context(), "filler.storage.library_budget_gb", "7"); err != nil {
		t.Fatal(err)
	}
	overrides := Overrides{DataDir: filepath.Join(root, "encryption")}
	build := func() foundationBuild {
		t.Helper()
		lifecycle := newGenerationLifecycle(t.Context())
		foundation, err := buildFoundation(t.Context(), st, slog.New(slog.DiscardHandler), overrides, lifecycle)
		if err != nil {
			t.Fatal(err)
		}
		decision := foundation.storageGovernor.Snapshot(t.Context(), foundation.fillerLayout.ClipDir())
		if !decision.Allowed || decision.Snapshot.SoftBudgetBytes != 7*storagegovernor.GiB || !decision.Snapshot.SoftLimitEnabled {
			t.Fatalf("storage projection = %+v", decision)
		}
		if err := lifecycle.shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		return foundation
	}
	_ = build()
	_ = build()
}

func TestFoundationRecomputesStorageForChangedFillerPathAfterRestart(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for path, size := range map[string]int{first: 100, second: 200} {
		if err := os.MkdirAll(filepath.Join(path, "_watch"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "clip.mp4"), make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	preparedDir := filepath.Join(root, "prepared")
	diagnosticsDir := filepath.Join(root, "diagnostics")
	for _, dir := range []string{preparedDir, diagnosticsDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PLAYOUT_PREPARED_DIR", preparedDir)
	t.Setenv("DIAGNOSTICS_DIR", diagnosticsDir)
	overrides := Overrides{DataDir: filepath.Join(root, "encryption")}
	project := func(wantPath string, wantManaged int64) {
		t.Helper()
		lifecycle := newGenerationLifecycle(t.Context())
		foundation, err := buildFoundation(t.Context(), st, slog.New(slog.DiscardHandler), overrides, lifecycle)
		if err != nil {
			t.Fatal(err)
		}
		if foundation.fillerLayout.ClipDir() != wantPath {
			t.Fatalf("applied filler path = %q, want %q", foundation.fillerLayout.ClipDir(), wantPath)
		}
		decision := foundation.storageGovernor.Snapshot(t.Context(), wantPath)
		if !decision.Allowed || decision.Snapshot.ManagedBytes != wantManaged {
			t.Fatalf("storage projection for %s = %+v", wantPath, decision)
		}
		if err := lifecycle.shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for _, setting := range []struct{ key, value string }{
		{key: "filler.dir", value: first}, {key: "filler.watch_dir", value: filepath.Join(first, "_watch")},
	} {
		if err := st.SetSetting(t.Context(), setting.key, setting.value); err != nil {
			t.Fatal(err)
		}
	}
	project(first, 100)
	for _, setting := range []struct{ key, value string }{
		{key: "filler.dir", value: second}, {key: "filler.watch_dir", value: filepath.Join(second, "_watch")},
	} {
		if err := st.SetSetting(t.Context(), setting.key, setting.value); err != nil {
			t.Fatal(err)
		}
	}
	project(second, 200)
}

func TestBuildRetainsStartupReportWhenSettingsInitializationFails(t *testing.T) {
	t.Setenv("JOB_WORKERS", "not-a-number")
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(100, 0)
	startup := diagnostics.NewStartup(now, 1, "v1", []diagnostics.StartupCheck{
		{Key: diagnostics.StartupCheckDatabase, Required: true},
		{Key: diagnostics.StartupCheckGeneratedSecrets, Required: true},
		{Key: diagnostics.StartupCheckHTTP, Required: true},
	}, func() time.Time { return now })
	startup.Complete(diagnostics.StartupCheckDatabase, diagnostics.StartupPassed, "ready", "", "")
	if _, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), Overrides{Startup: startup}); err == nil {
		t.Fatal("Build succeeded with invalid settings")
	}

	records, err := st.QueryDiagnosticEvents(context.Background(), diagnostics.EventStoreQuery{
		From: 1, To: time.Now().Add(time.Hour).UnixMilli(), Limit: 20, Event: "startup.complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("startup records = %+v", records)
	}
	var attributes struct {
		Report diagnostics.StartupReport `json:"report"`
	}
	if err := json.Unmarshal([]byte(records[0].AttributesJSON), &attributes); err != nil {
		t.Fatal(err)
	}
	if attributes.Report.State != diagnostics.StartupBlocked || attributes.Report.GenerationEnded == 0 {
		t.Fatalf("retained failed report = %+v", attributes.Report)
	}
}

func TestFoundationDiagnosticsFlushOnGenerationShutdown(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	lifecycle := newGenerationLifecycle(t.Context())
	foundation, err := buildFoundation(
		t.Context(), st, slog.New(slog.DiscardHandler), Overrides{}, lifecycle,
	)
	if err != nil {
		t.Fatalf("buildFoundation: %v", err)
	}
	if foundation.diagnostics == nil {
		t.Fatal("foundation diagnostics recorder is nil")
	}
	foundation.diagnostics.Record(t.Context(), diagnostics.Event{
		Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer,
		Subsystem: "app", Name: "application.test",
	})

	if err := lifecycle.shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	page, err := foundation.diagnosticEvents.Query(t.Context(), diagnostics.EventQuery{Limit: 10})
	if err != nil {
		t.Fatalf("query diagnostic events: %v", err)
	}
	found := false
	for _, event := range page.Items {
		found = found || event.Event == "application.test"
	}
	if !found {
		t.Fatalf("flushed records = %+v, want application.test", page.Items)
	}
}

func TestFoundationLoggerRedactsAndPersistsTheSameSlogRecord(t *testing.T) {
	const secret = "tmdb-secret-value-for-diagnostics"
	t.Setenv("TMDB_API_KEY", secret)
	st := testkit.MigratedSQLiteStore(t)
	lifecycle := newGenerationLifecycle(t.Context())
	var stdout bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&stdout, nil))
	foundation, err := buildFoundation(t.Context(), st, base, Overrides{}, lifecycle)
	if err != nil {
		t.Fatalf("buildFoundation: %v", err)
	}
	foundation.log.Info("configured "+secret,
		"event", "application.configured", "subsystem", "app", "token", secret)
	if err := lifecycle.shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if strings.Contains(stdout.String(), secret) || !strings.Contains(stdout.String(), "‹redacted›") {
		t.Fatalf("stdout redaction = %s", stdout.String())
	}
	records, err := st.ListDiagnosticEvents(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var configured *diagnostics.Record
	for i := range records {
		if records[i].Event == "application.configured" && records[i].Subsystem == "app" {
			configured = &records[i]
			break
		}
	}
	if configured == nil {
		t.Fatalf("persisted slog records = %+v", records)
	}
	if strings.Contains(configured.Message+configured.AttributesJSON, secret) ||
		!strings.Contains(configured.Message+configured.AttributesJSON, "‹redacted›") {
		t.Fatalf("durable redaction = %+v", configured)
	}
}
