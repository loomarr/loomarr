package store

import (
	"context"
	"database/sql"
	"testing"
)

// channelJobsMigration makes a channel and the suggestion jobs about it reference each other by
// foreign key (#2793).
const channelJobsMigration = 211

func TestChannelJobsMigration(t *testing.T) {
	t.Parallel()
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", sqliteDSNAt(t, channelJobsMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testChannelJobsMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testChannelJobsMigration proves the migration marks every channel made from a request as such,
// keeps each link that names a row, turns ” and a link naming no row into NULL, drops a job whose
// channel is gone, and that afterwards the links are enforced: a job goes with its channel, and a
// purged job leaves its channel with no intent_ref.
func testChannelJobsMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, channelJobsMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", channelJobsMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO channels (id, intent_ref, name, number, strategy, status) VALUES
			('ch-req', 'job-origin', 'Requested', 1, 'sequential', 'live'),
			('ch-purged', 'job-purged', 'Purged origin', 2, 'sequential', 'live'),
			('ch-hand', '', 'By hand', 3, 'sequential', 'live')`,
		`INSERT INTO proposal_jobs (id, status, channel_id) VALUES
			('job-origin', 'done', ''),
			('job-refine', 'done', 'ch-req'),
			('job-orphan', 'done', 'ch-gone')`,
		`INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at)
			VALUES ('job-orphan', 1, 1, 'succeeded', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", channelJobsMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, channelJobsMigration); err != nil {
		t.Fatalf("apply %d: %v", channelJobsMigration, err)
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
		`SELECT intent_ref FROM channels WHERE id = 'ch-req'`:                 "job-origin",
		`SELECT intent_ref FROM channels WHERE id = 'ch-purged'`:              "NULL",
		`SELECT intent_ref FROM channels WHERE id = 'ch-hand'`:                "NULL",
		`SELECT channel_id FROM proposal_jobs WHERE id = 'job-origin'`:        "NULL",
		`SELECT channel_id FROM proposal_jobs WHERE id = 'job-refine'`:        "ch-req",
		`SELECT count(*) FROM proposal_jobs WHERE id = 'job-orphan'`:          "0",
		`SELECT count(*) FROM proposal_job_attempts`:                          "0",
		`SELECT count(*) FROM channels`:                                       "3",
		`SELECT count(*) FROM channels WHERE from_request`:                    "2",
		`SELECT count(*) FROM channels WHERE id = 'ch-hand' AND from_request`: "0",
	} {
		if got := link(query); got != want {
			t.Errorf("%s = %s, want %s", query, got, want)
		}
	}

	for _, stmt := range []string{
		`INSERT INTO proposal_jobs (id, status, channel_id) VALUES ('job-nowhere', 'done', 'ch-none')`,
		`UPDATE channels SET intent_ref = 'job-none' WHERE id = 'ch-hand'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("accepted a link to no row; the foreign key is missing: %s", stmt)
		}
	}
	// A purged job leaves its channel with no intent_ref; a deleted channel takes its jobs.
	if _, err := db.ExecContext(ctx, `DELETE FROM proposal_jobs WHERE id = 'job-origin'`); err != nil {
		t.Fatalf("purge a channel's origin job: %v", err)
	}
	if got := link(`SELECT intent_ref FROM channels WHERE id = 'ch-req'`); got != "NULL" {
		t.Errorf("intent_ref after its job was purged = %s, want NULL", got)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM channels WHERE id = 'ch-req'`); err != nil {
		t.Fatalf("delete a channel: %v", err)
	}
	if got := link(`SELECT count(*) FROM proposal_jobs WHERE id = 'job-refine'`); got != "0" {
		t.Errorf("refine jobs after their channel was deleted = %s, want 0", got)
	}
}
