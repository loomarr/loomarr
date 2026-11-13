package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// proposalPeopleMigration makes the people on a suggestion foreign keys to users and moves the
// grant's decisions to decided_via (#2793).
const proposalPeopleMigration = 232

func TestProposalPeopleMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "proposal-people.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testProposalPeopleMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testProposalPeopleMigration proves the migration keeps each person who is a member, turns the
// API token's empty id and 'api-token' and an id naming no user into NULL, records how each decided
// suggestion was decided, and that afterwards the people are enforced and a deleted member is
// cleared from what they asked for, decided and said.
func testProposalPeopleMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, proposalPeopleMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", proposalPeopleMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name) VALUES ('u-kid', 'Kid'), ('u-parent', 'Parent')`,
		`INSERT INTO proposal_jobs (id, status, created_by) VALUES
			('job-kid', 'done', 'u-kid'), ('job-token', 'done', ''), ('job-gone', 'done', 'u-gone')`,
		`INSERT INTO proposals (id, job_id, status, created_by, decided_by) VALUES
			('p-person', 'job-kid', 'approved', 'u-kid', 'u-parent'),
			('p-auto', 'job-kid', 'approved', 'u-kid', 'auto'),
			('p-curate', 'job-kid', 'approved', 'u-kid', 'auto-curate'),
			('p-denied', 'job-kid', 'denied', 'u-kid', 'u-parent'),
			('p-token', 'job-token', 'approved', '', ''),
			('p-gone', 'job-gone', 'denied', 'u-gone', 'u-gone'),
			('p-waiting', 'job-kid', 'submitted', 'u-kid', ''),
			('p-no-job', '', 'denied', 'u-kid', 'u-parent'),
			('p-job-purged', 'job-purged', 'approved', 'u-kid', 'u-parent')`,
		`INSERT INTO discovery_feedback (id, actor_id, scope, target_key, action, created_at_ns) VALUES
			('f-kid', 'u-kid', 'household', 'movie:tmdb:90000001', 'keep', 1),
			('f-token', 'api-token', 'household', 'movie:tmdb:90000002', 'never', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", proposalPeopleMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, proposalPeopleMigration); err != nil {
		t.Fatalf("apply %d: %v", proposalPeopleMigration, err)
	}

	value := func(query string) string {
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
	check := func(want map[string]string) {
		t.Helper()
		for query, want := range want {
			if got := value(query); got != want {
				t.Errorf("%s = %s, want %s", query, got, want)
			}
		}
	}
	check(map[string]string{
		`SELECT created_by FROM proposal_jobs WHERE id = 'job-kid'`:                                         "u-kid",
		`SELECT created_by FROM proposal_jobs WHERE id = 'job-token'`:                                       "NULL",
		`SELECT created_by FROM proposal_jobs WHERE id = 'job-gone'`:                                        "NULL",
		`SELECT created_by || '/' || decided_by || '/' || decided_via FROM proposals WHERE id = 'p-person'`: "u-kid/u-parent/person",
		`SELECT decided_by FROM proposals WHERE id = 'p-auto'`:                                              "NULL",
		`SELECT decided_via FROM proposals WHERE id = 'p-auto'`:                                             "auto",
		`SELECT decided_by FROM proposals WHERE id = 'p-curate'`:                                            "NULL",
		`SELECT decided_via FROM proposals WHERE id = 'p-curate'`:                                           "auto-curate",
		`SELECT decided_via FROM proposals WHERE id = 'p-denied'`:                                           "person",
		`SELECT created_by FROM proposals WHERE id = 'p-token'`:                                             "NULL",
		`SELECT decided_by FROM proposals WHERE id = 'p-token'`:                                             "NULL",
		`SELECT decided_via FROM proposals WHERE id = 'p-token'`:                                            "person",
		`SELECT created_by FROM proposals WHERE id = 'p-gone'`:                                              "NULL",
		`SELECT decided_by FROM proposals WHERE id = 'p-gone'`:                                              "NULL",
		`SELECT decided_via FROM proposals WHERE id = 'p-waiting'`:                                          "NULL",
		`SELECT decided_by FROM proposals WHERE id = 'p-waiting'`:                                           "NULL",
		`SELECT actor_id FROM discovery_feedback WHERE id = 'f-kid'`:                                        "u-kid",
		`SELECT actor_id FROM discovery_feedback WHERE id = 'f-token'`:                                      "NULL",
		`SELECT job_id FROM proposals WHERE id = 'p-person'`:                                                "job-kid",
		`SELECT job_id FROM proposals WHERE id = 'p-no-job'`:                                                "NULL",
		`SELECT job_id FROM proposals WHERE id = 'p-job-purged'`:                                            "NULL",
		`SELECT count(*) FROM proposals`:                                                                    "9",
	})
	if _, err := db.ExecContext(ctx, `UPDATE proposals SET job_id = 'job-none' WHERE id = 'p-no-job'`); err == nil {
		t.Error("accepted a suggestion naming a job that does not exist")
	}
	// The purge of a finished job leaves its decided suggestions with no job named.
	if _, err := db.ExecContext(ctx, `DELETE FROM proposal_jobs WHERE id = 'job-token'`); err != nil {
		t.Fatalf("purge a finished job: %v", err)
	}
	check(map[string]string{`SELECT job_id FROM proposals WHERE id = 'p-token'`: "NULL"})

	for _, stmt := range []string{
		`INSERT INTO proposal_jobs (id, status, created_by) VALUES ('job-nobody', 'done', 'u-none')`,
		`UPDATE proposals SET created_by = 'u-none' WHERE id = 'p-token'`,
		`UPDATE proposals SET decided_by = 'u-none' WHERE id = 'p-token'`,
		`UPDATE proposals SET decided_via = 'grant' WHERE id = 'p-token'`,
		`UPDATE discovery_feedback SET actor_id = 'u-none' WHERE id = 'f-token'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("accepted a person who is no member, or an unknown decided_via: %s", stmt)
		}
	}

	// A deleted member leaves what they asked for, decided and said, with no one named.
	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id IN ('u-kid', 'u-parent')`); err != nil {
		t.Fatalf("delete members: %v", err)
	}
	check(map[string]string{
		`SELECT created_by FROM proposal_jobs WHERE id = 'job-kid'`:                             "NULL",
		`SELECT count(*) FROM proposals WHERE created_by IS NOT NULL OR decided_by IS NOT NULL`: "0",
		`SELECT decided_via FROM proposals WHERE id = 'p-person'`:                               "person",
		`SELECT actor_id FROM discovery_feedback WHERE id = 'f-kid'`:                            "NULL",
		`SELECT count(*) FROM proposals`:                                                        "9",
	})
}
