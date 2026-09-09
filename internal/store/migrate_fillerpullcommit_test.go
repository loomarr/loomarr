package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestFillerPullCommitMigrationPreservesHistoricalRuns(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "pull-upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testFillerPullCommitMigration(t, db, DialectSQLite, "migrations/sqlite")
}

func testFillerPullCommitMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), 101); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO filler_pulls (id, title, created_at) VALUES ('old-pull', 'Historical audit', 1)`,
		`INSERT INTO filler_acquisition_runs (id, trigger, pull_id, started_at, updated_at) VALUES ('old-one', 'pull', 'old-pull', 1, 1)`,
		`INSERT INTO filler_acquisition_runs (id, trigger, pull_id, started_at, updated_at) VALUES ('old-two', 'pull', 'old-pull', 2, 2)`,
	} {
		if _, err := db.ExecContext(t.Context(), query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(t.Context(), 102); err != nil {
		t.Fatalf("upgrade with historical duplicate runs: %v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM filler_acquisition_runs WHERE pull_id = 'old-pull'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("historical runs after upgrade = %d (%v), want both audit records", count, err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM filler_pull_commits`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("upgrade invented new approval bindings: %d (%v)", count, err)
	}
}
