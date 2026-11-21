package store

import (
	"context"
	"testing"
)

// A library scanned before #2151 decision 60 holds `extra` file rows, which the scan no longer
// writes. Migration 187's kind CHECK must drop them rather than fail, or such an install can't
// start after the upgrade.
func TestIntegrityChecksDropStaleExtraFileRowsSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, sqliteFileAt(t, 186, storeConnections))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 186); err != nil {
		t.Fatalf("migrate through 186: %v", err)
	}
	for path, kind := range map[string]string{
		"/media/film.mkv":         "movie",
		"/media/film-trailer.mkv": "extra",
		"/media/odd.iso":          "",
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO library_files
			(path, root, size_bytes, modified_ns, kind, seen_at_ms) VALUES (?, '/media', 1, 1, ?, 1)`,
			path, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(ctx, 187); err != nil {
		t.Fatalf("apply the integrity checks over a stale extra row: %v", err)
	}
	var kept, extras int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_files`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_files WHERE kind = 'extra'`).Scan(&extras); err != nil {
		t.Fatal(err)
	}
	if kept != 2 || extras != 0 {
		t.Fatalf("after 187: %d rows, %d extras; want the film and the problem file kept, the extra gone", kept, extras)
	}
}
