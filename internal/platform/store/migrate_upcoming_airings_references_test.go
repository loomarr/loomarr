package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// upcomingAiringsReferenceMigration makes the library item an upcoming airing names a foreign key
// (#2793).
const upcomingAiringsReferenceMigration = 260

func TestUpcomingAiringsReferenceMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "upcoming-references.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testUpcomingAiringsReferenceMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testUpcomingAiringsReferenceMigration proves the migration keeps every upcoming airing that
// names a library item and drops one that names none, and that afterwards the reference is
// enforced and an airing goes with its item.
func testUpcomingAiringsReferenceMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, upcomingAiringsReferenceMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", upcomingAiringsReferenceMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms) VALUES ('lib-film', 'movie', 'Film', 'film', 1)`,
		`INSERT INTO channels (id, name, number, strategy, status, desired_json, policy_json) VALUES ('ch1', 'Cartoons', 3, 'shuffle', 'live', '[]', '{}')`,
		`INSERT INTO channel_upcoming_airings (channel_id, starts_at_ms, ends_at_ms, library_item_id, title_key, forecast, revision) VALUES
			('ch1', 10, 20, 'lib-film', 'movie:1', FALSE, 1),
			('ch1', 20, 30, '90000934', 'movie:3', TRUE, 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", upcomingAiringsReferenceMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, upcomingAiringsReferenceMigration); err != nil {
		t.Fatalf("apply %d: %v", upcomingAiringsReferenceMigration, err)
	}

	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_upcoming_airings`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count(); got != 1 {
		t.Fatalf("rows after the migration = %d, want only the airing of lib-film", got)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO channel_upcoming_airings
		(channel_id, starts_at_ms, ends_at_ms, library_item_id, title_key, forecast, revision)
		VALUES ('ch1', 40, 50, '90000935', 'movie:4', FALSE, 1)`); err == nil {
		t.Error("an airing of no item was accepted, want a foreign key violation")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM library_items WHERE id = 'lib-film'`); err != nil {
		t.Fatalf("delete an item only upcoming airings name: %v", err)
	}
	if got := count(); got != 0 {
		t.Errorf("rows after their item went = %d, want 0", got)
	}
}
