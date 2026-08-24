package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/testkit"
)

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
