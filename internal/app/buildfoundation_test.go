package app

import (
	"log/slog"
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
	records, err := st.ListDiagnosticEvents(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListDiagnosticEvents: %v", err)
	}
	if len(records) != 1 || records[0].Event != "application.test" {
		t.Fatalf("flushed records = %+v, want application.test", records)
	}
}
