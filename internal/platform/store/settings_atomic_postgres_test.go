//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// TestPostgresSettingBatchIsReplicaAtomic proves the cross-replica property that
// an in-process fake cannot: while one replica is blocked partway through a batch,
// another sees the complete prior generation, never a tuple assembled from both.
func TestPostgresSettingBatchIsReplicaAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := startPostgres(t)

	writerStore, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writerStore.Close() }()
	readerStore, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readerStore.Close() }()

	old := SettingBatch{Upserts: []SettingMutation{
		{Key: "library.flavor", Value: "emby"},
		{Key: "library.url", Value: "http://old:8096"},
		{Key: "library.token", Value: "old-token"},
	}}
	if err := writerStore.ApplySettingBatch(ctx, old); err != nil {
		t.Fatal(err)
	}

	writer := writerStore.(*sqlStore)
	blocker, err := writer.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	var locked string
	if err := blocker.QueryRowContext(ctx,
		`SELECT key FROM settings WHERE key = $1 FOR UPDATE`, "library.url").Scan(&locked); err != nil {
		t.Fatal(err)
	}

	batchDone := make(chan error, 1)
	go func() {
		batchDone <- writerStore.ApplySettingBatch(ctx, SettingBatch{Upserts: []SettingMutation{
			// The first write executes before the lock wait. Without a surrounding
			// transaction, a reader would observe this new flavor with the old URL/token.
			{Key: "library.flavor", Value: "plex"},
			{Key: "library.url", Value: "http://new:32400"},
			{Key: "library.token", Value: "new-token"},
		}})
	}()

	awaitBlockedSettingBatch(t, ctx, readerStore.(*sqlStore))
	assertSettingValues(t, ctx, readerStore, map[string]string{
		"library.flavor": "emby",
		"library.url":    "http://old:8096",
		"library.token":  "old-token",
	})

	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-batchDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatalf("setting batch did not finish after lock release: %v", ctx.Err())
	}
	assertSettingValues(t, ctx, readerStore, map[string]string{
		"library.flavor": "plex",
		"library.url":    "http://new:32400",
		"library.token":  "new-token",
	})
}

func awaitBlockedSettingBatch(t *testing.T, ctx context.Context, observer *sqlStore) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		err := observer.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database()
				  AND wait_event_type = 'Lock'
				  AND query LIKE 'INSERT INTO settings%'
			)`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("setting batch never blocked on the guarded middle row: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func assertSettingValues(t *testing.T, ctx context.Context, s Store, want map[string]string) {
	t.Helper()
	rows, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(rows))
	for _, row := range rows {
		got[row.Key] = row.Value
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q (settings snapshot: %+v)", key, got[key], value, got)
		}
	}
}
