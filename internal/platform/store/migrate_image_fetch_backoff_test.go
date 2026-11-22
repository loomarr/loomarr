package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Migration 00248 adds the fetch-failure record with no failures for every existing row, the
// adopted fetch class, and the queue time as each row's creation: an image still waiting for its
// download stays on the fetch queue in the order it had, and a fetched one stays off it.
func TestImageFetchBackoffMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, sqliteFileAt(t, 247, storeConnections))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testImageFetchBackoffMigration(t, s, "migrations/sqlite")
}

func testImageFetchBackoffMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 247); err != nil {
		t.Fatalf("migrate through 247: %v", err)
	}
	at := time.Unix(1_700_000_000, 0).Unix()
	pending, fetched := strings.Repeat("a1", 32), strings.Repeat("b2", 32)
	for hash, fetchedAt := range map[string]int64{pending: 0, fetched: at} {
		if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO images
			(hash, origin, source_url, role, mime, origin_fetched_at, created_at, updated_at, last_used_at)
			VALUES (?, 'remote', ?, 'poster', 'image/jpeg', ?, ?, ?, ?)`),
			hash, "https://image.example.test/"+hash[:4]+".jpg", fetchedAt, at, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(ctx, 248); err != nil {
		t.Fatalf("apply 248: %v", err)
	}

	queue, err := s.ListImagesAwaitingFetch(ctx, time.Unix(at, 0), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Hash != pending {
		t.Fatalf("queue after 248 = %d rows, want just the image still waiting", len(queue))
	}
	for _, hash := range []string{pending, fetched} {
		img, err := s.GetImage(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if img.FetchFailures != 0 || !img.FetchFailedAt.IsZero() || !img.FetchRetryAt.IsZero() {
			t.Fatalf("%s after 248 = %d failures, failed %v, retry %v; want none", hash[:4], img.FetchFailures,
				img.FetchFailedAt, img.FetchRetryAt)
		}
		if img.FetchQueuedAt.Unix() != at || img.FetchPriority != 0 {
			t.Fatalf("%s after 248 joined the queue at %v with priority %d, want its creation and the adopted class",
				hash[:4], img.FetchQueuedAt, img.FetchPriority)
		}
	}
}
