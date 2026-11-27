package store

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
)

// integrityLeafChecksMigration rebuilds the unreferenced tables with CHECKs on their state columns
// and indexes the foreign keys that lacked one (#2528).
const integrityLeafChecksMigration = 187

var integrityLeafRebuilt = []string{
	"activity", "diagnostic_process_runs", "filler_acquisition_artifacts", "filler_clip_defects",
	"filler_clip_pipeline", "filler_curated_offers", "filler_enrichment_axes", "filler_ready_events",
	"interactive_operations", "library_answers", "library_candidates", "library_catalog",
	"library_files", "library_filmography", "library_id_map", "library_markers", "library_streams",
	"quality_daily_aggregates", "quality_receipts",
}

// The rebuild keeps every column exactly (name, type, nullability, default, key), every index and
// foreign key, and every row, including the ” values two columns write on purpose; afterwards a
// misspelt state is refused.
func TestIntegrityLeafChecksMigration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sql.Open("sqlite", sqliteDSNAt(t, integrityLeafChecksMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, integrityLeafChecksMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", integrityLeafChecksMigration-1, err)
	}
	before := map[string]string{}
	for _, table := range integrityLeafRebuilt {
		before[table] = tableShape(t, db, table)
	}
	// The one column the migration drops: it only ever held 'ready'.
	before["filler_ready_events"] = strings.Replace(before["filler_ready_events"], "outcome TEXT 1 'ready' 0; ", "", 1)
	// activity.kind loses the '' default its CHECK would refuse.
	before["activity"] = strings.Replace(before["activity"], "kind TEXT 1 '' 0", "kind TEXT 1 NULL 0", 1)
	for _, stmt := range []string{
		`INSERT INTO activity (id, at, kind, level, text) VALUES ('a1', 1, 'channel', 'warn', 'moved')`,
		`INSERT INTO library_files (path, root, size_bytes, modified_ns, kind, seen_at_ms) VALUES ('/m/a.mkv', '/m', 1, 1, '', 1)`,
		`INSERT INTO filler_curated_offers (source_id, list_version, offered_at) VALUES ('s1', 'v1', 1)`,
		`INSERT INTO interactive_operations (id, kind, subject, status, started_at, updated_at)
			VALUES ('op1', 'llm_pull', 'model', 'success', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", integrityLeafChecksMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, integrityLeafChecksMigration); err != nil {
		t.Fatalf("apply %d: %v", integrityLeafChecksMigration, err)
	}

	for _, table := range integrityLeafRebuilt {
		after := tableShape(t, db, table)
		for _, added := range []string{"idx_filler_acquisition_artifacts_acquisition 0 0; ", "idx_quality_receipts_run_snapshot 0 0"} {
			after = strings.Replace(after, added, "", 1) // the foreign-key indexes the migration adds
		}
		after = strings.TrimSuffix(after, "; ")
		if after != before[table] {
			t.Errorf("%s changed shape:\nbefore %s\nafter  %s", table, before[table], after)
		}
	}
	for query, want := range map[string]string{
		`SELECT kind FROM library_files WHERE path = '/m/a.mkv'`:               "",
		`SELECT local_state FROM filler_curated_offers WHERE source_id = 's1'`: "",
		`SELECT status FROM interactive_operations WHERE id = 'op1'`:           "success",
	} {
		var got string
		if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q kept", query, got, want)
		}
	}
	for _, stmt := range []string{
		`UPDATE library_files SET kind = 'film' WHERE path = '/m/a.mkv'`,
		`UPDATE filler_curated_offers SET local_state = 'ON' WHERE source_id = 's1'`,
		`UPDATE interactive_operations SET status = 'succeeded' WHERE id = 'op1'`,
		`UPDATE activity SET kind = 'channel.reconcile' WHERE id = 'a1'`,
		`UPDATE activity SET level = 'debug' WHERE id = 'a1'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "CHECK") {
			t.Errorf("%s: err = %v, want a CHECK failure", stmt, err)
		}
	}
}

// tableShape is a table's columns, foreign keys and index definitions in one comparable string.
func tableShape(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var parts []string
	for _, query := range []string{
		`SELECT name || ' ' || type || ' ' || "notnull" || ' ' || coalesce(dflt_value, 'NULL') || ' ' || pk FROM pragma_table_info(?1)`,
		`SELECT "table" || '.' || coalesce("to", '') || ' ' || "from" || ' ' || on_delete FROM pragma_foreign_key_list(?1)`,
		`SELECT name || ' ' || "unique" || ' ' || partial FROM pragma_index_list(?1) WHERE origin = 'c'`,
	} {
		rows, err := db.Query(query, table)
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		slices.Sort(lines)
		parts = append(parts, strings.Join(lines, "; "))
	}
	return strings.Join(parts, " | ")
}
