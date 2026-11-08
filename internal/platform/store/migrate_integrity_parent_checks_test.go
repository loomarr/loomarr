package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// integrityParentChecksMigration rebuilds the referenced tables, with every descendant, so their
// state columns carry CHECKs (#2528).
const integrityParentChecksMigration = 185

var integrityParentRebuilt = []string{
	"channels", "user_favourite_channels", "user_recent_channels", "airings",
	"filler_acquisition_runs", "filler_pulls", "filler_acquisition_artifacts", "filler_pull_commits",
	"filler_sources", "filler_source_check_outcomes",
	"inventory_sources", "inventory_source_origins", "inventory_source_measurements", "inventory_source_analysis",
	"library_items", "library_ids", "library_genres", "library_studios", "library_tags", "library_sources",
	"library_streams", "library_trickplay", "library_chapters", "library_markers", "library_art",
	"library_countries", "library_credits",
	"proposal_jobs", "proposal_job_attempts",
}

// A rebuild that left a child pointing at the renamed parent, or cascaded through it, loses the
// child's rows: this seeds a parent with a child (and a grandchild under library_items), applies the
// migration with foreign keys on as production opens SQLite, and proves the rows stayed, every
// table kept its shape, deleting a parent still cascades, and a misspelt state is refused.
func TestIntegrityParentChecksMigration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "integrity.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, integrityParentChecksMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", integrityParentChecksMigration-1, err)
	}
	before := map[string]string{}
	for _, table := range integrityParentRebuilt {
		before[table] = tableShape(t, db, table)
	}
	// The one column the migration drops: nothing ever set it.
	before["channels"] = strings.Replace(before["channels"], "filler_ref TEXT 1 '' 0; ", "", 1)
	for _, stmt := range []string{
		`INSERT INTO channels (id, name, number, strategy, status) VALUES ('ch1', 'Cartoons', 3, 'shuffle', 'live')`,
		`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at) VALUES ('ch1', 'ep1', 1, 1)`,
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms) VALUES ('m1', 'movie', 'Film', 'film', 1)`,
		`INSERT INTO library_sources (item_id, path) VALUES ('m1', '/m/film.mkv')`,
		`INSERT INTO library_streams (item_id, path, stream_index, kind) VALUES ('m1', '/m/film.mkv', 0, 'video')`,
		`INSERT INTO proposal_jobs (id, kind, status) VALUES ('j1', 'recurate', 'done')`,
		`INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at)
			VALUES ('j1', 1, 1, 'succeeded', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", integrityParentChecksMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, integrityParentChecksMigration); err != nil {
		t.Fatalf("apply %d: %v", integrityParentChecksMigration, err)
	}

	for _, table := range integrityParentRebuilt {
		if after := tableShape(t, db, table); after != before[table] {
			t.Errorf("%s changed shape:\nbefore %s\nafter  %s", table, before[table], after)
		}
	}
	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM airings WHERE channel_id = 'ch1'`:                  1,
		`SELECT count(*) FROM library_streams WHERE item_id = 'm1'`:              1,
		`SELECT count(*) FROM proposal_job_attempts WHERE job_id = 'j1'`:         1,
		`SELECT count(*) FROM pragma_foreign_key_check`:                          0,
		`SELECT count(*) FROM sqlite_master WHERE name LIKE '%\_old' ESCAPE '\'`: 0,
	} {
		if got := count(query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	for _, stmt := range []string{
		`UPDATE channels SET status = 'on_air' WHERE id = 'ch1'`,
		`UPDATE library_items SET kind = 'film' WHERE id = 'm1'`,
		`UPDATE proposal_jobs SET kind = 'curate' WHERE id = 'j1'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "CHECK") {
			t.Errorf("%s: err = %v, want a CHECK failure", stmt, err)
		}
	}
	// Deleting a parent still reaches its new children, two levels down.
	for _, stmt := range []string{`DELETE FROM channels WHERE id = 'ch1'`, `DELETE FROM library_items WHERE id = 'm1'`} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(`SELECT (SELECT count(*) FROM airings) + (SELECT count(*) FROM library_streams)`); n != 0 {
		t.Errorf("%d child rows outlived their deleted parents", n)
	}
}
