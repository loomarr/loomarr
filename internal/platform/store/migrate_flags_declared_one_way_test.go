package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
)

// flagsDeclaredOneWayMigration rebuilds every table with a flag, with every descendant, so each
// flag is BOOLEAN NOT NULL CHECK (col IN (0, 1)) and images.meta is CHECKed as JSON (#2793).
const flagsDeclaredOneWayMigration = 191

// flagsRebuilt maps each rebuilt table to its flags.
var flagsRebuilt = map[string][]string{
	"users":                        {"disabled", "auto_approve", "media_server_linked"},
	"sessions":                     nil,
	"device_pairings":              nil,
	"device_tokens":                nil,
	"password_recoveries":          nil,
	"password_recovery_grants":     nil,
	"user_hidden_ideas":            nil,
	"client_crash_reports":         nil,
	"user_favourite_channels":      nil,
	"user_recent_channels":         nil,
	"images":                       {"animated"},
	"image_derivatives":            {"animated"},
	"image_refs":                   nil,
	"clips":                        {"held", "is_composite"},
	"filler_research_web_attempts": nil,
	"clip_tags":                    {"leaf"},
	"filler_sources":               {"enabled"},
	"filler_source_check_outcomes": nil,
	"filler_clip_pipeline":         {"force_run"},
	"holidays":                     {"name_needs_corroboration"},
	"library_answers":              {"ignored"},
	"library_candidates":           {"name_match", "year_match", "length_match"},
	"library_files":                {"held"},
	"library_root_paths":           {"by_hand"},
	"library_roots":                {"in_use", "readable", "by_hand"},
	"library_items":                {"grounded", "needs_noticed"},
	"library_ids":                  nil,
	"library_genres":               nil,
	"library_studios":              nil,
	"library_tags":                 nil,
	"library_sources":              {"plays"},
	"library_art":                  nil,
	"library_countries":            nil,
	"library_credits":              nil,
	"library_streams":              {"is_default", "forced", "commentary", "hearing_impaired", "descriptive"},
	"library_trickplay":            nil,
	"library_chapters":             nil,
	"library_markers":              nil,
	"proposal_jobs":                {"reached_live"},
	"proposal_job_attempts":        nil,
	"quality_run_snapshots":        {"accounting_available"},
	"quality_receipts":             nil,
	"scheduled_jobs":               {"paused"},
	"settings":                     {"env_override"},
	"filler_acquisition_artifacts": {"provider_archive_committed"},
	"filler_providers":             {"enabled"},
	"notification_destinations":    {"enabled"},
	"secret_data_keys":             {"active"},
}

// A rebuild that left a child pointing at the renamed parent, or cascaded through it, loses the
// child's rows, and one that reordered a copy's columns scrambles them under INSERT ... SELECT *:
// this seeds parent rows, their child rows and grandchild rows, applies the migration with foreign keys on as
// production opens SQLite, and proves every table kept its columns in order, every flag became a
// checked BOOLEAN with its value, the rows stayed, deletes still cascade, and a 2 is refused.
func TestFlagsDeclaredOneWayMigration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sql.Open("sqlite", sqliteDSNAt(t, flagsDeclaredOneWayMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, flagsDeclaredOneWayMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", flagsDeclaredOneWayMigration-1, err)
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
		if _, ok := flagsRebuilt[parent]; ok {
			if _, ok := flagsRebuilt[child]; !ok {
				t.Errorf("%s references %s, which %d rebuilds: rebuild %s with it", child, parent,
					flagsDeclaredOneWayMigration, child)
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
	for table, flags := range flagsRebuilt {
		beforeShape[table] = asFlags(tableShape(t, db, table), flags)
		beforeOrder[table] = order(table)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name, disabled, auto_approve, media_server_linked) VALUES ('u1', 'Sam', 1, 0, 1)`,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('t1', 'u1', 99)`,
		`INSERT INTO password_recoveries (id, user_id, status, created_at, expires_at) VALUES ('r1', 'u1', 'pending', 1, 1801)`,
		`INSERT INTO password_recovery_grants (token_hash, recovery_id, created_at, expires_at)
			VALUES ('` + strings.Repeat("a", 64) + `', 'r1', 1, 2)`,
		`INSERT INTO images (hash, origin, mime, animated, meta) VALUES ('i1', 'upload', 'image/png', 1, '{"b":1, "a":2}')`,
		`INSERT INTO image_derivatives (image_hash, recipe, format, width, path, animated) VALUES ('i1', 'w', 'webp', 64, '/d', 1)`,
		`INSERT INTO clips (hash, path, name, role, duration_ms, held, updated_at) VALUES ('c1', 'c1.mp4', 'Ad', 'ad', 1000, 1, 1)`,
		`INSERT INTO filler_research_web_attempts (clip_hash, input_revision, adapter_version, provider, reserved_at)
			VALUES ('c1', 1, 'v1', 'brave', 1)`,
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms, grounded) VALUES ('m1', 'movie', 'Film', 'film', 1, 0)`,
		`INSERT INTO library_sources (item_id, path, plays) VALUES ('m1', '/m/film.mkv', 1)`,
		`INSERT INTO library_streams (item_id, path, stream_index, kind, forced) VALUES ('m1', '/m/film.mkv', 0, 'audio', 1)`,
		`INSERT INTO proposal_jobs (id, status, reached_live) VALUES ('j1', 'done', 1)`,
		`INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at)
			VALUES ('j1', 1, 1, 'succeeded', 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", flagsDeclaredOneWayMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, flagsDeclaredOneWayMigration); err != nil {
		t.Fatalf("apply %d: %v", flagsDeclaredOneWayMigration, err)
	}

	for table := range flagsRebuilt {
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
		`SELECT count(*) FROM users WHERE disabled = 1 AND auto_approve = 0 AND media_server_linked = 1`: 1,
		`SELECT count(*) FROM sessions WHERE user_id = 'u1' AND expires_at = 99`:                         1,
		`SELECT count(*) FROM password_recovery_grants WHERE recovery_id = 'r1'`:                         1,
		`SELECT count(*) FROM images WHERE animated = 1 AND meta = '{"b":1, "a":2}'`:                     1,
		`SELECT count(*) FROM image_derivatives WHERE animated = 1`:                                      1,
		`SELECT count(*) FROM clips WHERE held = 1 AND duration_ms = 1000`:                               1,
		`SELECT count(*) FROM filler_research_web_attempts WHERE clip_hash = 'c1'`:                       1,
		`SELECT count(*) FROM library_items WHERE grounded = 0`:                                          1,
		`SELECT count(*) FROM library_streams WHERE forced = 1 AND kind = 'audio'`:                       1,
		`SELECT count(*) FROM proposal_job_attempts WHERE job_id = 'j1'`:                                 1,
		`SELECT count(*) FROM pragma_foreign_key_check`:                                                  0,
		`SELECT count(*) FROM sqlite_master WHERE name LIKE '%\_old' ESCAPE '\'`:                         0,
	} {
		if got := count(query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	for _, stmt := range []string{
		`UPDATE users SET disabled = 2 WHERE id = 'u1'`,
		`UPDATE clips SET held = -1 WHERE hash = 'c1'`,
		`UPDATE library_streams SET forced = 2`,
		`UPDATE images SET meta = '{not json' WHERE hash = 'i1'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "CHECK") {
			t.Errorf("%s: err = %v, want a CHECK failure", stmt, err)
		}
	}
	// Deleting a parent still reaches its new children, two levels down.
	for _, stmt := range []string{`DELETE FROM users WHERE id = 'u1'`, `DELETE FROM library_items WHERE id = 'm1'`,
		`DELETE FROM images WHERE hash = 'i1'`, `DELETE FROM clips WHERE hash = 'c1'`} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(`SELECT (SELECT count(*) FROM sessions) + (SELECT count(*) FROM password_recovery_grants)
		+ (SELECT count(*) FROM library_streams) + (SELECT count(*) FROM image_derivatives)
		+ (SELECT count(*) FROM filler_research_web_attempts)`); n != 0 {
		t.Errorf("%d child rows outlived their deleted parents", n)
	}
}

// asFlags rewrites a tableShape as the migration should leave it: each flag column BOOLEAN with a
// FALSE or TRUE default (or none).
func asFlags(shape string, flags []string) string {
	columns, rest, _ := strings.Cut(shape, " | ")
	parts := strings.Split(columns, "; ")
	for i, p := range parts {
		f := strings.Fields(p) // name type notnull default pk
		if len(f) != 5 || !slices.Contains(flags, f[0]) {
			continue
		}
		switch strings.ToLower(f[3]) {
		case "0", "false":
			f[3] = "FALSE"
		case "1", "true":
			f[3] = "TRUE"
		}
		f[1] = "BOOLEAN"
		parts[i] = strings.Join(f, " ")
	}
	return strings.Join(parts, "; ") + " | " + rest
}
