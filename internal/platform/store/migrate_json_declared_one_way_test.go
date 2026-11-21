package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
)

// jsonDeclaredOneWayMigration rebuilds every table with a JSON column, with every descendant, so
// each JSON column is CHECKed with json_valid and "no document" is NULL rather than ” (#2793).
const jsonDeclaredOneWayMigration = 192

// jsonRebuilt maps each rebuilt table to its columns that held ” for "no document": they become
// nullable, with no default.
var jsonRebuilt = map[string][]string{
	"channels":                            {"intent_json"},
	"airings":                             nil,
	"scheduled_airings":                   nil,
	"user_favourite_channels":             nil,
	"user_recent_channels":                nil,
	"client_crash_reports":                nil,
	"diagnostic_events":                   nil,
	"filler_clip_defects":                 {"judgement_json"},
	"filler_clip_fingerprints":            nil,
	"filler_clip_pipeline":                nil,
	"filler_context_reports":              nil,
	"filler_enrichment_axes":              nil,
	"filler_split_proposals":              nil,
	"filler_split_shadow_decisions":       nil,
	"filler_inference_evaluations":        nil,
	"filler_structure_window_call_ledger": {"record_json"},
	"filler_pulls":                        nil,
	"filler_pull_commits":                 nil,
	"inventory_item_origins":              nil,
	"inventory_external_ids":              nil,
	"inventory_source_origins":            nil,
	"inventory_source_analysis":           nil,
	"inventory_source_measurements":       nil,
	"notification_destinations":           nil,
	"notification_intents":                nil,
	"notification_delivery_attempts":      nil,
	"proposal_jobs":                       {"failure_trace_json"},
	"proposal_job_attempts":               nil,
	"proposals":                           nil,
	"titles":                              nil,
	"series_episodes":                     nil,
	"taxa":                                nil,
	"holidays":                            {"air_window"},
}

// A rebuild that left a child pointing at the renamed parent loses the child's rows, and one that
// reordered a copy's columns scrambles them under INSERT ... SELECT *: this seeds rows (with ”
// where a document was absent), applies the migration with foreign keys on as production opens
// SQLite, and proves every table kept its columns in order and its shape apart from the absent
// documents, each ” became NULL, the rows stayed, deletes still cascade, and text that is not JSON
// is refused.
func TestJSONDeclaredOneWayMigration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sql.Open("sqlite", sqliteDSNAt(t, jsonDeclaredOneWayMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, jsonDeclaredOneWayMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", jsonDeclaredOneWayMigration-1, err)
	}
	// A table that references a rebuilt one must be rebuilt with it, or dropping the renamed parent
	// cascades through its rows. A migration that adds such a child before this one fails here.
	rows, err := db.QueryContext(ctx, `SELECT m.name, f."table" FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			t.Fatal(err)
		}
		if _, ok := jsonRebuilt[parent]; ok {
			if _, ok := jsonRebuilt[child]; !ok {
				t.Errorf("%s references %s, which %d rebuilds: rebuild %s with it", child, parent,
					jsonDeclaredOneWayMigration, child)
			}
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatal(err)
	}
	order := func(table string) string {
		t.Helper()
		var cols string
		if err := db.QueryRowContext(ctx, `SELECT group_concat(name, ',') FROM (SELECT name FROM pragma_table_info(?1) ORDER BY cid)`,
			table).Scan(&cols); err != nil {
			t.Fatal(err)
		}
		return cols
	}
	beforeShape, beforeOrder := map[string]string{}, map[string]string{}
	for table, absent := range jsonRebuilt {
		beforeShape[table] = asOptional(tableShape(t, db, table), absent)
		beforeOrder[table] = order(table)
	}
	for _, stmt := range []string{
		`INSERT INTO channels (id, name, number, strategy, status, lineup_json, intent_json)
			VALUES ('ch1', 'Cartoons', 3, 'shuffle', 'live', '[{"id":"m1"}]', '')`,
		`INSERT INTO channels (id, name, number, strategy, status, intent_json)
			VALUES ('ch2', 'Films', 4, 'shuffle', 'live', '{"ask":"films"}')`,
		`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at) VALUES ('ch1', 'ep1', 1, 1)`,
		`INSERT INTO proposal_jobs (id, status, failure_trace_json) VALUES ('j1', 'done', '')`,
		`INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at)
			VALUES ('j1', 1, 1, 'succeeded', 1)`,
		`INSERT INTO taxa (slug, label, axis) VALUES ('halloween', 'Halloween', 'holiday')`,
		`INSERT INTO holidays (id, label, position, air_window) VALUES ('halloween', 'Halloween', 1, '')`,
		`INSERT INTO filler_clip_defects (clip_hash, measurement_version, floor_version, era, outcome,
			readings_json, assessment_json, measured_at, judgement_json)
			VALUES ('c1', 'v', 'v', 1990, 'pass', '{}', '{}', 1, '')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", jsonDeclaredOneWayMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, jsonDeclaredOneWayMigration); err != nil {
		t.Fatalf("apply %d: %v", jsonDeclaredOneWayMigration, err)
	}

	for table := range jsonRebuilt {
		if after := order(table); after != beforeOrder[table] {
			t.Errorf("%s reordered its columns:\nbefore %s\nafter  %s", table, beforeOrder[table], after)
		}
		if after := tableShape(t, db, table); after != beforeShape[table] {
			t.Errorf("%s changed shape:\nwant  %s\nafter %s", table, beforeShape[table], after)
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
		`SELECT count(*) FROM channels WHERE id = 'ch1' AND intent_json IS NULL AND lineup_json = '[{"id":"m1"}]'`: 1,
		`SELECT count(*) FROM channels WHERE id = 'ch2' AND intent_json = '{"ask":"films"}'`:                       1,
		`SELECT count(*) FROM airings WHERE channel_id = 'ch1'`:                                                    1,
		`SELECT count(*) FROM proposal_jobs WHERE failure_trace_json IS NULL`:                                      1,
		`SELECT count(*) FROM proposal_job_attempts WHERE job_id = 'j1'`:                                           1,
		`SELECT count(*) FROM holidays WHERE air_window IS NULL`:                                                   1,
		`SELECT count(*) FROM filler_clip_defects WHERE judgement_json IS NULL`:                                    1,
		`SELECT count(*) FROM pragma_foreign_key_check`:                                                            0,
		`SELECT count(*) FROM sqlite_master WHERE name LIKE '%\_old' ESCAPE '\'`:                                   0,
	} {
		if got := count(query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	for _, stmt := range []string{
		`UPDATE channels SET lineup_json = '' WHERE id = 'ch1'`,
		`UPDATE channels SET intent_json = '{bad' WHERE id = 'ch2'`,
		`UPDATE proposal_jobs SET failure_trace_json = '' WHERE id = 'j1'`,
		`UPDATE holidays SET air_window = 'every October'`,
		`UPDATE taxa SET synonyms = 'spooky'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "CHECK") {
			t.Errorf("%s: err = %v, want a CHECK failure", stmt, err)
		}
	}
	// Deleting a parent still reaches its new children.
	for _, stmt := range []string{`DELETE FROM channels WHERE id = 'ch1'`, `DELETE FROM proposal_jobs WHERE id = 'j1'`} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(`SELECT (SELECT count(*) FROM airings) + (SELECT count(*) FROM proposal_job_attempts)`); n != 0 {
		t.Errorf("%d child rows outlived their deleted parents", n)
	}
}

// asOptional rewrites a tableShape as the migration should leave it: each column that held ” for
// "no document" nullable, with no default.
func asOptional(shape string, absent []string) string {
	columns, rest, _ := strings.Cut(shape, " | ")
	parts := strings.Split(columns, "; ")
	for i, p := range parts {
		f := strings.Fields(p) // name type notnull default pk
		if len(f) != 5 || !slices.Contains(absent, f[0]) {
			continue
		}
		f[2], f[3] = "0", "NULL"
		parts[i] = strings.Join(f, " ")
	}
	return strings.Join(parts, "; ") + " | " + rest
}
