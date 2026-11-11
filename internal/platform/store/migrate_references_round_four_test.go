package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// referencesRoundFourMigration keys a setting's author, a download's pull, and the download a
// clip's conveyor and ready event name (#2793).
const referencesRoundFourMigration = 226

func TestReferencesRoundFourMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "round-four.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testReferencesRoundFourMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testReferencesRoundFourMigration proves the migration keeps every row, keeps each name of a row
// that exists, turns '' and a name of a row that is gone into NULL, and that afterwards the keys
// are enforced: a person's or a pull's delete forgets them, and a download a clip names is kept.
func testReferencesRoundFourMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, referencesRoundFourMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", referencesRoundFourMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name) VALUES ('90000941', 'Ana')`,
		`INSERT INTO settings (key, value, updated_by) VALUES
			('by-person', 'a', '90000941'), ('by-system', 'b', 'system'), ('by-no-one', 'c', NULL)`,
		`INSERT INTO filler_pulls (id, title, status, created_at) VALUES ('90000942', 'Pull', 'approved', 1)`,
		`INSERT INTO filler_acquisition_runs (id, trigger, pull_id, started_at, updated_at) VALUES
			('90000943', 'pull', '90000942', 1, 1), ('90000944', 'pull', '90000949', 1, 1), ('90000945', 'source', '', 1, 1)`,
		`INSERT INTO filler_clip_pipeline (clip_hash, acquisition_id, enrolled_at, updated_at) VALUES
			('clip-from-run', '90000943', 1, 1), ('clip-from-folder', '', 1, 1), ('clip-from-gone-run', '90000948', 1, 1)`,
		`INSERT INTO filler_ready_events (id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref, placement, created_at) VALUES
			('ready-1', 'clip-from-run', '90000943', 'acquisition', '90000943', 'pool', 1),
			('ready-2', 'clip-from-folder', '', 'source', 'folder', 'pool', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", referencesRoundFourMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, referencesRoundFourMigration); err != nil {
		t.Fatalf("apply %d: %v", referencesRoundFourMigration, err)
	}

	for _, c := range []struct {
		query string
		want  map[string]sql.NullString
	}{
		{`SELECT key, updated_by FROM settings`, map[string]sql.NullString{
			"by-person": {String: "90000941", Valid: true}, "by-system": {}, "by-no-one": {},
		}},
		{`SELECT id, pull_id FROM filler_acquisition_runs`, map[string]sql.NullString{
			"90000943": {String: "90000942", Valid: true}, "90000944": {}, "90000945": {},
		}},
		{`SELECT clip_hash, acquisition_id FROM filler_clip_pipeline`, map[string]sql.NullString{
			"clip-from-run": {String: "90000943", Valid: true}, "clip-from-folder": {}, "clip-from-gone-run": {},
		}},
		{`SELECT id, acquisition_id FROM filler_ready_events`, map[string]sql.NullString{
			"ready-1": {String: "90000943", Valid: true}, "ready-2": {},
		}},
	} {
		got := map[string]sql.NullString{}
		rows, err := db.QueryContext(ctx, c.query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id string
			var ref sql.NullString
			if err := rows.Scan(&id, &ref); err != nil {
				t.Fatal(err)
			}
			got[id] = ref
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		if len(got) != len(c.want) {
			t.Errorf("%s: %d rows, want %d (every row kept)", c.query, len(got), len(c.want))
		}
		for id, want := range c.want {
			if got[id] != want {
				t.Errorf("%s: %s = %v, want %v", c.query, id, got[id], want)
			}
		}
	}

	// Enforced from now on.
	for _, stmt := range []string{
		`INSERT INTO settings (key, value, updated_by) VALUES ('by-stranger', 'd', '90000946')`,
		`INSERT INTO filler_acquisition_runs (id, trigger, pull_id, started_at, updated_at) VALUES ('90000947', 'pull', '90000946', 1, 1)`,
		`INSERT INTO filler_clip_pipeline (clip_hash, acquisition_id, enrolled_at, updated_at) VALUES ('clip-x', '90000946', 1, 1)`,
		`INSERT INTO filler_ready_events (id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref, placement, created_at)
			VALUES ('ready-x', 'clip-x', '90000946', 'acquisition', 'x', 'pool', 1)`,
		`DELETE FROM filler_acquisition_runs WHERE id = '90000943'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s: accepted, want a foreign key violation", stmt)
		}
	}
	// A person's or a pull's delete forgets them; the setting and the run stay.
	for _, stmt := range []string{
		`DELETE FROM users WHERE id = '90000941'`,
		`DELETE FROM filler_pulls WHERE id = '90000942'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	var author, pull sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT updated_by FROM settings WHERE key = 'by-person'`).Scan(&author); err != nil || author.Valid {
		t.Errorf("setting after its author's delete: updated_by = %v, %v; want the row kept with NULL", author, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT pull_id FROM filler_acquisition_runs WHERE id = '90000943'`).Scan(&pull); err != nil || pull.Valid {
		t.Errorf("run after its pull's delete: pull_id = %v, %v; want the row kept with NULL", pull, err)
	}
}
