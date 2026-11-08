package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

const quarantineLegacyFillerMigration = 101

func TestQuarantineLegacyFillerMigration_SQLite(t *testing.T) {
	t.Parallel()
	s, err := openSQLite(context.Background(), filepath.Join(t.TempDir(), "quarantine.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testQuarantineLegacyFillerMigration(t, s.db, DialectSQLite, "migrations/sqlite")
}

// testQuarantineLegacyFillerMigration applies the shipped migration to the schema it was written
// for (through 00100), seeded with a deliberately old catalog. Running it at head would replay SQL
// against columns later migrations dropped (#2323), so the fixture speaks the 00100 schema in raw
// SQL rather than the current store API.
func testQuarantineLegacyFillerMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	bind := func(query string) string {
		if dialect == DialectPostgres {
			return storedb.PgPlaceholders(query)
		}
		return query
	}
	if _, err := provider.UpTo(ctx, quarantineLegacyFillerMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", quarantineLegacyFillerMigration-1, err)
	}
	for _, seed := range []struct {
		hash            string
		held, composite bool
		removedAt       int64
		disposition     string
	}{
		{hash: "legacy-playable", disposition: "filed"},
		{hash: "legacy-without-pipeline"},
		{hash: "already-held", held: true, disposition: "review"},
		{hash: "composite", composite: true, disposition: "filed"},
		{hash: "removed", removedAt: 1_700_000_000, disposition: "filed"},
	} {
		// Every row carries the retired publisher's physical marker. The migration clears it
		// everywhere even though the application no longer read the column.
		if _, err := db.ExecContext(ctx, bind(`INSERT INTO clips
			(hash, path, name, kind, duration_ms, held, is_composite, removed_at, auto_filed, updated_at)
			VALUES (?, ?, ?, 'commercial', 30000, ?, ?, ?, ?, 1)`),
			seed.hash, seed.hash+".mp4", seed.hash, seed.held, seed.composite, seed.removedAt, true); err != nil {
			t.Fatalf("seed clip %s: %v", seed.hash, err)
		}
		if seed.disposition == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, bind(`INSERT INTO filler_clip_pipeline
			(clip_hash, stage, status, progress, disposition, enrolled_at, updated_at)
			VALUES (?, 'finish', 'done', 100, ?, 1, 1)`), seed.hash, seed.disposition); err != nil {
			t.Fatalf("seed pipeline %s: %v", seed.hash, err)
		}
	}
	if _, err := provider.UpTo(ctx, quarantineLegacyFillerMigration); err != nil {
		t.Fatalf("apply %d: %v", quarantineLegacyFillerMigration, err)
	}

	held := func(hash string) bool {
		t.Helper()
		var v bool
		if err := db.QueryRowContext(ctx, bind(`SELECT held FROM clips WHERE hash = ?`), hash).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, hash := range []string{"legacy-playable", "legacy-without-pipeline", "already-held"} {
		if !held(hash) {
			t.Errorf("%s is not held, want quarantined", hash)
		}
	}
	for _, hash := range []string{"composite", "removed"} {
		if held(hash) {
			t.Errorf("%s was held, want its lifecycle unchanged", hash)
		}
	}
	for hash, want := range map[string]string{
		"legacy-playable": "review", "already-held": "review", "composite": "filed", "removed": "filed",
	} {
		var disposition, status string
		if err := db.QueryRowContext(ctx, bind(`SELECT disposition, status FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).
			Scan(&disposition, &status); err != nil {
			t.Fatal(err)
		}
		if disposition != want || status != "done" {
			t.Errorf("%s pipeline = %s/%s, want %s/done", hash, disposition, status, want)
		}
	}
	var marked int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clips WHERE auto_filed = TRUE`).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if marked != 0 {
		t.Errorf("legacy auto_filed markers = %d, want 0", marked)
	}
}
