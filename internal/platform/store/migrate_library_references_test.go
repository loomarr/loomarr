package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// libraryReferencesMigration makes the library items a title and an airing name foreign keys
// (#2793).
const libraryReferencesMigration = 221

func TestLibraryReferencesMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "library-references.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testLibraryReferencesMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testLibraryReferencesMigration proves the migration keeps every title and airing that names a
// library item, turns a title's empty or dangling library id into NULL, drops an airing that names
// no item, and that afterwards the references are enforced and an item they name can't be deleted.
func testLibraryReferencesMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, libraryReferencesMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", libraryReferencesMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms) VALUES ('lib-film', 'movie', 'Film', 'film', 1)`,
		`INSERT INTO channels (id, name, number, strategy, status) VALUES ('ch1', 'Cartoons', 3, 'shuffle', 'live')`,
		`INSERT INTO titles (key, title_json, state, library_id) VALUES
			('movie:1', '{}', 'available', 'lib-film'),
			('movie:2', '{}', 'wanted', ''),
			('movie:3', '{}', 'available', '90000921')`,
		`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at, key) VALUES
			('ch1', 'lib-film', 10, 10, 'movie:1'),
			('ch1', '90000922', 20, 20, 'movie:3')`,
		`INSERT INTO scheduled_airings (channel_id, library_item_id, aired_at, recorded_at, key) VALUES
			('ch1', 'lib-film', 10, 10, 'movie:1'),
			('ch1', '90000923', 20, 20, 'movie:3')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", libraryReferencesMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, libraryReferencesMigration); err != nil {
		t.Fatalf("apply %d: %v", libraryReferencesMigration, err)
	}

	titles := map[string]sql.NullString{}
	rows, err := db.QueryContext(ctx, `SELECT key, library_id FROM titles`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		var id sql.NullString
		if err := rows.Scan(&key, &id); err != nil {
			t.Fatal(err)
		}
		titles[key] = id
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if got := titles["movie:1"]; !got.Valid || got.String != "lib-film" {
		t.Errorf("title naming an item: library_id = %v, want lib-film", got)
	}
	for _, key := range []string{"movie:2", "movie:3"} {
		if got, ok := titles[key]; !ok || got.Valid {
			t.Errorf("title %s: library_id = %v (present %v), want the row kept with NULL", key, got, ok)
		}
	}
	for _, table := range []string{"airings", "scheduled_airings"} {
		var n int
		var item string
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(library_item_id) FROM `+table).Scan(&n, &item); err != nil {
			t.Fatal(err)
		}
		if n != 1 || item != "lib-film" {
			t.Errorf("%s: %d rows, first naming %q; want only the airing of lib-film", table, n, item)
		}
	}

	// Enforced from now on.
	for _, stmt := range []string{
		`INSERT INTO titles (key, title_json, state, library_id) VALUES ('movie:4', '{}', 'available', '90000924')`,
		`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at) VALUES ('ch1', '90000925', 30, 30)`,
		`INSERT INTO scheduled_airings (channel_id, library_item_id, aired_at, recorded_at) VALUES ('ch1', '90000926', 30, 30)`,
		`DELETE FROM library_items WHERE id = 'lib-film'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s: accepted, want a foreign key violation", stmt)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO titles (key, title_json, state) VALUES ('movie:5', '{}', 'wanted')`); err != nil {
		t.Errorf("a title not in the library: %v", err)
	}
}
