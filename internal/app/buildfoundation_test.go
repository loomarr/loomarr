package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/testkit"
)

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
	if len(page.Items) != 1 || page.Items[0].Event != "application.test" {
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
	if len(records) != 1 || records[0].Event != "application.configured" || records[0].Subsystem != "app" {
		t.Fatalf("persisted slog records = %+v", records)
	}
	if strings.Contains(records[0].Message+records[0].AttributesJSON, secret) ||
		!strings.Contains(records[0].Message+records[0].AttributesJSON, "‹redacted›") {
		t.Fatalf("durable redaction = %+v", records[0])
	}
}
