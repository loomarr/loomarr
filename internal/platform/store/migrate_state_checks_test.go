package store

import (
	"context"
	"database/sql"
	"testing"
)

// stateChecksMigration is the migration that puts CHECK constraints on the core state columns
// (#2323).
const stateChecksMigration = 166

func TestStateChecksMigration(t *testing.T) {
	t.Parallel()
	// Foreign keys on, as production opens SQLite: a rebuild that left the attempts pointing at a
	// dropped table, or cascaded through it, fails here.
	db, err := sql.Open("sqlite", sqliteDSNAt(t, stateChecksMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testStateChecksMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testStateChecksMigration proves the constraints keep every state the code writes, including a
// superseded Proposal (written only by SQL, never named in Go), and refuse anything else. On
// SQLite it also proves the rebuilt attempts table still points at the rebuilt jobs table.
func testStateChecksMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, stateChecksMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", stateChecksMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO titles (key, title_json, state) VALUES ('t-kept', '{}', 'unavailable')`,
		`INSERT INTO proposals (id, job_id, status) VALUES ('p-kept', 'j-kept', 'superseded')`,
		`INSERT INTO proposal_jobs (id, status) VALUES ('j-kept', 'done')`,
		`INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at)
			VALUES ('j-kept', 1, 1, 'succeeded', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", stateChecksMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, stateChecksMigration); err != nil {
		t.Fatalf("apply %d: %v", stateChecksMigration, err)
	}

	for query, want := range map[string]string{
		`SELECT state FROM titles WHERE key = 't-kept'`:                    "unavailable",
		`SELECT status FROM proposals WHERE id = 'p-kept'`:                 "superseded",
		`SELECT status FROM proposal_jobs WHERE id = 'j-kept'`:             "done",
		`SELECT status FROM proposal_job_attempts WHERE job_id = 'j-kept'`: "succeeded",
	} {
		var got string
		if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q kept", query, got, want)
		}
	}
	if dialect == DialectSQLite {
		var parent string
		if err := db.QueryRowContext(ctx,
			`SELECT "table" FROM pragma_foreign_key_list('proposal_job_attempts')`).Scan(&parent); err != nil {
			t.Fatal(err)
		}
		if parent != "proposal_jobs" {
			t.Errorf("proposal_job_attempts references %q, want proposal_jobs", parent)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM proposal_jobs WHERE id = 'j-kept'`); err != nil {
		t.Fatal(err)
	}
	var orphaned int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM proposal_job_attempts WHERE job_id = 'j-kept'`).Scan(&orphaned); err != nil {
		t.Fatal(err)
	}
	if orphaned != 0 {
		t.Errorf("deleting a job left %d attempts; the cascade did not survive the rebuild", orphaned)
	}

	for _, stmt := range []string{
		`INSERT INTO titles (key, title_json, state) VALUES ('t-bad', '{}', 'availabel')`,
		`INSERT INTO proposals (id, status) VALUES ('p-bad', 'declined')`,
		`INSERT INTO proposal_jobs (id, status) VALUES ('j-bad', 'cancelled')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("accepted an unknown state: %s", stmt)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO titles (key, title_json, state) VALUES ('t-w', '{}', 'wanted'), ('t-r', '{}', 'requested'),
			('t-d', '{}', 'downloading'), ('t-a', '{}', 'available')`,
		`INSERT INTO proposals (id, status) VALUES ('p-s', 'submitted'), ('p-a', 'approved'), ('p-d', 'denied')`,
		`INSERT INTO proposal_jobs (id, status) VALUES ('j-q', 'queued'), ('j-r', 'running'), ('j-f', 'failed')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Errorf("refused a known state: %v", err)
		}
	}
}
