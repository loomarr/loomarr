//go:build integration

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestTrackedPostgresReplicaSettingsRefreshObservesOtherStoreBatch(t *testing.T) {
	stores := testkit.PostgresStores(t, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	old := store.SettingBatch{Upserts: []store.SettingMutation{
		{Key: "library.flavor", Value: "emby"},
		{Key: "library.url", Value: "http://old:8096"},
		{Key: "library.token", Value: "old-token"},
	}}
	if err := stores[0].ApplySettingBatch(ctx, old); err != nil {
		t.Fatal(err)
	}
	baseLog := slog.New(slog.NewTextHandler(io.Discard, nil))
	protection := testSecretProtection(t, stores[1])
	set, _, _, _, err := bootSettings(ctx, stores[1], protection, baseLog)
	if err != nil {
		t.Fatal(err)
	}
	owner := newGenerationLifecycle(ctx)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := owner.shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown settings lifecycle: %v", err)
		}
	})
	if !trackReplicaSettingsRefresh(owner, store.DialectOf(stores[1]), set.svc, 5*time.Millisecond, nil) {
		t.Fatal("Postgres did not start its settings refresher")
	}

	batchAt := time.Unix(1_700_000_000, 0).UTC()
	if err := (storePersister{st: stores[0], protection: protection}).Apply(ctx, settings.PersistenceBatch{
		Upserts: []settings.PersistedSetting{
			{Key: "library.flavor", Value: "jellyfin"},
			{Key: "library.url", Value: "http://new:8096"},
			{Key: "library.token", Value: "new-token"},
		}, UpdatedAt: batchAt, UpdatedBy: "replica-a",
	}); err != nil {
		t.Fatal(err)
	}
	storedToken, err := stores[0].GetSetting(ctx, "library.token")
	if err != nil || storedToken == "new-token" || !secretprotection.IsEnvelope(storedToken) {
		t.Fatalf("Postgres stored token = (%q, %v), want encrypted envelope", storedToken, err)
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn := set.libraryConnection()
		if conn.Flavor == "jellyfin" && conn.BaseURL == "http://new:8096" && conn.Token == "new-token" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("replica kept stale connection %+v: %v", conn, ctx.Err())
		case <-ticker.C:
		}
	}
}
