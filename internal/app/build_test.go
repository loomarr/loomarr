package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

func buildTestApplication(t *testing.T, st store.Store, overrides Overrides) *Application {
	t.Helper()
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
