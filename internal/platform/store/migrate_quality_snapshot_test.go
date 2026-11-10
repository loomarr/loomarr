package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// qualitySnapshotMigration makes a quality daily total name its evaluation run by foreign key
// (#2793).
const qualitySnapshotMigration = 223

func TestQualitySnapshotMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "quality-snapshot.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testQualitySnapshotMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testQualitySnapshotMigration proves the migration keeps a total that names a run, turns ” into
// NULL, drops a total naming a run that is gone, and that afterwards the run is enforced, a run a
// total names can't be deleted, and ordinary use still has one total per day, stage and outcome.
func testQualitySnapshotMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, qualitySnapshotMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", qualitySnapshotMigration-1, err)
	}
	const totals = `INSERT INTO quality_daily_aggregates (day, stage, outcome, run_snapshot_id,
		observation_count, duration_ms, tool_calls, candidate_count, cost_nanos) VALUES `
	for _, stmt := range []string{
		`INSERT INTO quality_run_snapshots (id, schema_version, corpus_version, requested_model, resolved_model,
			provider, budget_profile, application_version, accounting_available, created_at)
			VALUES ('run-kept', 1, 'c', 'm', 'm', 'p', 'b', 'v', TRUE, 1)`,
		totals + `('2026-11-01', 'retrieval', 'succeeded', 'run-kept', 1, 1, 1, 1, 1),
			('2026-11-01', 'retrieval', 'succeeded', '', 2, 2, 2, 2, 2),
			('2026-11-01', 'retrieval', 'succeeded', '90000941', 3, 3, 3, 3, 3)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", qualitySnapshotMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, qualitySnapshotMigration); err != nil {
		t.Fatalf("apply %d: %v", qualitySnapshotMigration, err)
	}

	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	for _, c := range []struct {
		what  string
		query string
		want  int
	}{
		{"total naming a run", `SELECT count(*) FROM quality_daily_aggregates WHERE run_snapshot_id = 'run-kept'`, 1},
		{"ordinary total", `SELECT count(*) FROM quality_daily_aggregates WHERE run_snapshot_id IS NULL AND observation_count = 2`, 1},
		{"total naming a gone run", `SELECT count(*) FROM quality_daily_aggregates WHERE observation_count = 3`, 0},
		{"empty run ids", `SELECT count(*) FROM quality_daily_aggregates WHERE run_snapshot_id = ''`, 0},
	} {
		if got := count(c.query); got != c.want {
			t.Errorf("%s: %d rows, want %d", c.what, got, c.want)
		}
	}

	// Enforced from now on.
	for _, stmt := range []string{
		totals + `('2026-11-02', 'retrieval', 'succeeded', '90000942', 1, 1, 1, 1, 1)`,
		totals + `('2026-11-01', 'retrieval', 'succeeded', NULL, 1, 1, 1, 1, 1)`, // a second ordinary total
		`DELETE FROM quality_run_snapshots WHERE id = 'run-kept'`,                // a total names it
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s: accepted, want it refused", stmt)
		}
	}

	// The rollup's upsert adds to the ordinary total rather than making a second one.
	if _, err := db.ExecContext(ctx, totals+`('2026-11-01', 'retrieval', 'succeeded', NULL, 5, 5, 5, 5, 5)
		ON CONFLICT (day, stage, outcome, COALESCE(run_snapshot_id, '')) DO UPDATE SET
		observation_count = quality_daily_aggregates.observation_count + excluded.observation_count`); err != nil {
		t.Fatalf("upsert ordinary total: %v", err)
	}
	if got := count(`SELECT observation_count FROM quality_daily_aggregates WHERE run_snapshot_id IS NULL`); got != 7 {
		t.Errorf("ordinary total after upsert = %d, want 7", got)
	}
}
