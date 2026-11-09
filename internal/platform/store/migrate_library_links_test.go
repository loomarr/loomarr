package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// libraryLinksMigration makes the library's links between its own rows foreign keys (#2793).
const libraryLinksMigration = 209

func TestLibraryLinksMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "library-links.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testLibraryLinksMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testLibraryLinksMigration proves the migration keeps every link that names a row, turns '' and
// a link naming no row into NULL, drops a candidate whose title is gone, carries a title's
// descendants across whole, and that afterwards the links are enforced.
func testLibraryLinksMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, libraryLinksMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", libraryLinksMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms, series_id) VALUES
			('lib-show', 'series', 'Show', 'show', 1, ''),
			('lib-ep', 'episode', 'Pilot', 'pilot', 1, 'lib-show'),
			('lib-stray', 'episode', 'Stray', 'stray', 1, '90000901'),
			('lib-film', 'movie', 'Film', 'film', 1, '')`,
		`INSERT INTO library_ids (item_id, namespace, value) VALUES ('lib-film', 'tmdb', '90000902')`,
		`INSERT INTO library_sources (item_id, path) VALUES ('lib-film', '/m/film.mkv')`,
		`INSERT INTO library_streams (item_id, path, stream_index, kind) VALUES ('lib-film', '/m/film.mkv', 0, 'video')`,
		`INSERT INTO library_collections (id, name, created_at_ms, updated_at_ms) VALUES ('c1', 'Kept', 1, 1)`,
		`INSERT INTO library_collection_members (collection_id, inventory_item_id, position) VALUES ('c1', 'lib-film', 1)`,
		`INSERT INTO library_files (path, root, size_bytes, modified_ns, seen_at_ms, inventory_item_id, inventory_series_id)
			VALUES ('/m/film.mkv', '/m', 1, 1, 1, 'lib-film', ''),
			       ('/m/show/e1.mkv', '/m', 1, 1, 1, 'lib-ep', 'lib-show'),
			       ('/m/stray.mkv', '/m', 1, 1, 1, '90000903', '90000904'),
			       ('/m/new.mkv', '/m', 1, 1, 1, '', '')`,
		`INSERT INTO library_candidates (inventory_item_id, rank, kind, external_id, name)
			VALUES ('lib-film', 0, 'movie', '90000905', 'Guess'), ('90000906', 0, 'movie', '90000907', 'Orphan')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", libraryLinksMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, libraryLinksMigration); err != nil {
		t.Fatalf("apply %d: %v", libraryLinksMigration, err)
	}

	link := func(query string) string {
		t.Helper()
		var v sql.NullString
		if err := db.QueryRowContext(ctx, query).Scan(&v); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if !v.Valid {
			return "NULL"
		}
		return v.String
	}
	for query, want := range map[string]string{
		`SELECT series_id FROM library_items WHERE id = 'lib-ep'`:                         "lib-show",
		`SELECT series_id FROM library_items WHERE id = 'lib-stray'`:                      "NULL",
		`SELECT series_id FROM library_items WHERE id = 'lib-film'`:                       "NULL",
		`SELECT inventory_item_id FROM library_files WHERE path = '/m/film.mkv'`:          "lib-film",
		`SELECT inventory_series_id FROM library_files WHERE path = '/m/film.mkv'`:        "NULL",
		`SELECT inventory_series_id FROM library_files WHERE path = '/m/show/e1.mkv'`:     "lib-show",
		`SELECT inventory_item_id FROM library_files WHERE path = '/m/stray.mkv'`:         "NULL",
		`SELECT inventory_series_id FROM library_files WHERE path = '/m/stray.mkv'`:       "NULL",
		`SELECT inventory_item_id FROM library_files WHERE path = '/m/new.mkv'`:           "NULL",
		`SELECT inventory_item_id FROM library_candidates WHERE external_id = '90000905'`: "lib-film",
	} {
		if got := link(query); got != want {
			t.Errorf("%s = %s, want %s", query, got, want)
		}
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM library_items`:              4,
		`SELECT count(*) FROM library_files`:              4,
		`SELECT count(*) FROM library_candidates`:         1,
		`SELECT count(*) FROM library_ids`:                1,
		`SELECT count(*) FROM library_sources`:            1,
		`SELECT count(*) FROM library_streams`:            1,
		`SELECT count(*) FROM library_collection_members`: 1,
	} {
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if n != want {
			t.Errorf("%s = %d, want %d", query, n, want)
		}
	}

	for _, stmt := range []string{
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms, series_id)
			VALUES ('lib-ep2', 'episode', 'Two', 'two', 1, '90000908')`,
		`INSERT INTO library_files (path, root, size_bytes, modified_ns, seen_at_ms, inventory_item_id)
			VALUES ('/m/x.mkv', '/m', 1, 1, 1, '90000909')`,
		`INSERT INTO library_candidates (inventory_item_id, rank, kind, external_id, name)
			VALUES ('90000910', 0, 'movie', '90000911', 'Nobody')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("accepted a link to no row; the foreign key is missing: %s", stmt)
		}
	}
	// A title's candidates go with it.
	if _, err := db.ExecContext(ctx, `DELETE FROM library_files`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM library_collection_members`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM library_items WHERE id = 'lib-film'`); err != nil {
		t.Fatalf("delete a title nothing names: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM library_candidates`).Scan(&n); err != nil || n != 0 {
		t.Errorf("candidates after their title was deleted = %d, %v; want 0", n, err)
	}
}
