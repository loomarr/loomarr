package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

func buildTestApplication(t *testing.T, st store.Store, overrides Overrides) *Application {
	t.Helper()
	// PostgreSQL has no database-file directory from which secret protection can derive its
	// generated installation-key path. Keep every composition test isolated from the production
	// /data default and from other tests' key material.
	if overrides.EncryptionDataDir == "" {
		overrides.EncryptionDataDir = t.TempDir()
	}
	application, err := Build(t.Context(), st, slog.New(slog.DiscardHandler), overrides)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := application.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return application
}
