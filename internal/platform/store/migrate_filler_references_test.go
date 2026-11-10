package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// fillerReferencesMigration makes the filler download records' sources, pulls and runs foreign
// keys (#2793).
const fillerReferencesMigration = 222

func TestFillerReferencesMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "filler-references.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testFillerReferencesMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testFillerReferencesMigration proves the migration keeps every link that names a row, turns ''
// and a link naming a deleted source into NULL, and that afterwards the links are enforced:
// deleting a source or pull forgets it, a run a clip pipeline names can't be deleted, and a ready
// event forgets a deleted run.
func testFillerReferencesMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, fillerReferencesMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", fillerReferencesMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO filler_sources (id, kind, uri, created_at) VALUES ('src-kept', 'archive', 'kept', 1)`,
		`INSERT INTO filler_pulls (id, title, created_at) VALUES ('pull-kept', 'Pull', 1)`,
		`INSERT INTO filler_acquisition_runs (id, trigger, source_id, pull_id, started_at, updated_at) VALUES
			('run-kept', 'pull', 'src-kept', 'pull-kept', 1, 1),
			('run-gone-source', 'source', '90000931', '', 1, 1),
			('run-none', 'manual', '', '', 1, 1)`,
		`INSERT INTO filler_clip_pipeline (clip_hash, acquisition_id, enrolled_at, updated_at) VALUES
			('clip-from-run', 'run-kept', 1, 1), ('clip-by-hand', '', 1, 1)`,
		`INSERT INTO filler_ready_events (id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref, placement, created_at) VALUES
			('ready-run', 'clip-ready-run', 'run-none', 'source', 'src-kept', 'pool', 1),
			('ready-folder', 'clip-ready-folder', '', 'folder', 'f', 'pool', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", fillerReferencesMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, fillerReferencesMigration); err != nil {
		t.Fatalf("apply %d: %v", fillerReferencesMigration, err)
	}

	link := func(query string) sql.NullString {
		t.Helper()
		var v sql.NullString
		if err := db.QueryRowContext(ctx, query).Scan(&v); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return v
	}
	runLink := func(column, id string) sql.NullString {
		return link(`SELECT ` + column + ` FROM filler_acquisition_runs WHERE id = '` + id + `'`)
	}
	for _, c := range []struct {
		what string
		got  sql.NullString
		want string // "" means NULL
	}{
		{"kept source", runLink("source_id", "run-kept"), "src-kept"},
		{"kept pull", runLink("pull_id", "run-kept"), "pull-kept"},
		{"deleted source", runLink("source_id", "run-gone-source"), ""},
		{"no source", runLink("source_id", "run-none"), ""},
		{"no pull", runLink("pull_id", "run-none"), ""},
		{"pipeline run", link(`SELECT acquisition_id FROM filler_clip_pipeline WHERE clip_hash = 'clip-from-run'`), "run-kept"},
		{"pipeline by hand", link(`SELECT acquisition_id FROM filler_clip_pipeline WHERE clip_hash = 'clip-by-hand'`), ""},
		{"ready run", link(`SELECT acquisition_id FROM filler_ready_events WHERE id = 'ready-run'`), "run-none"},
		{"ready folder", link(`SELECT acquisition_id FROM filler_ready_events WHERE id = 'ready-folder'`), ""},
	} {
		if c.got.Valid != (c.want != "") || c.got.String != c.want {
			t.Errorf("%s = %v, want %q (\"\" is NULL)", c.what, c.got, c.want)
		}
	}

	// Enforced from now on.
	for _, stmt := range []string{
		`INSERT INTO filler_acquisition_runs (id, trigger, source_id, started_at, updated_at) VALUES ('run-x', 'source', '90000932', 1, 1)`,
		`INSERT INTO filler_acquisition_runs (id, trigger, pull_id, started_at, updated_at) VALUES ('run-y', 'pull', '90000933', 1, 1)`,
		`INSERT INTO filler_clip_pipeline (clip_hash, acquisition_id, enrolled_at, updated_at) VALUES ('clip-x', '90000934', 1, 1)`,
		`INSERT INTO filler_ready_events (id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref, placement, created_at)
			VALUES ('ready-x', 'clip-x', '90000935', 'folder', 'f', 'pool', 1)`,
		`DELETE FROM filler_acquisition_runs WHERE id = 'run-kept'`, // a pipeline row names it
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s: accepted, want a foreign key violation", stmt)
		}
	}
	for _, stmt := range []string{
		`DELETE FROM filler_sources WHERE id = 'src-kept'`,
		`DELETE FROM filler_pulls WHERE id = 'pull-kept'`,
		`DELETE FROM filler_acquisition_runs WHERE id = 'run-none'`, // a ready event names it
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for _, c := range []struct {
		what string
		got  sql.NullString
	}{
		{"source after delete", runLink("source_id", "run-kept")},
		{"pull after delete", runLink("pull_id", "run-kept")},
		{"ready run after delete", link(`SELECT acquisition_id FROM filler_ready_events WHERE id = 'ready-run'`)},
	} {
		if c.got.Valid {
			t.Errorf("%s = %q, want NULL", c.what, c.got.String)
		}
	}
}
