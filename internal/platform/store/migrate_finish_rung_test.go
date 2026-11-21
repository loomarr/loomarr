package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// finishRungMigration renames the terminal pipeline rung `score` → `finish` (#2323).
const finishRungMigration = 169

func TestFinishRungMigration(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", sqliteDSNAt(t, finishRungMigration-1, foreignKeyConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testFinishRungMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testFinishRungMigration proves both the current stage and every ladder record move, the ladder
// keeps its order and its other fields, and rows with no score record are left byte-for-byte.
func testFinishRungMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	rebindFor := func(dialect Dialect, query string) string {
		if dialect == DialectPostgres {
			return storedb.PgPlaceholders(query)
		}
		return query
	}
	if _, err := provider.UpTo(ctx, finishRungMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", finishRungMigration-1, err)
	}
	const published = `[{"stage":"probe","status":"done","at":"2026-11-01T00:00:00Z"},` +
		`{"stage":"vision","status":"skipped","note":"vision is off","at":"2026-11-01T00:00:01Z"},` +
		`{"stage":"score","status":"done","attempts":2,"at":"2026-11-01T00:00:02Z"}]`
	const midway = `[{"stage":"probe","status":"done","at":"2026-11-01T00:00:00Z"}]`
	for _, seed := range []struct{ hash, stage, ladder string }{
		{"published", "score", published},
		{"midway", "transcode", midway},
		{"empty", "probe", "[]"},
	} {
		if _, err := db.ExecContext(ctx, rebindFor(dialect,
			`INSERT INTO filler_clip_pipeline (clip_hash, stage, stages_json, enrolled_at, updated_at)
			VALUES (?, ?, ?, 1, 1)`),
			seed.hash, seed.stage, seed.ladder); err != nil {
			t.Fatalf("seed %s: %v", seed.hash, err)
		}
	}
	if _, err := provider.UpTo(ctx, finishRungMigration); err != nil {
		t.Fatalf("apply %d: %v", finishRungMigration, err)
	}

	read := func(hash string) (string, []map[string]any, string) {
		t.Helper()
		var stage, raw string
		if err := db.QueryRowContext(ctx, rebindFor(dialect,
			`SELECT stage, stages_json FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).
			Scan(&stage, &raw); err != nil {
			t.Fatal(err)
		}
		var ladder []map[string]any
		if err := json.Unmarshal([]byte(raw), &ladder); err != nil {
			t.Fatalf("%s ladder is not JSON: %v (%s)", hash, err, raw)
		}
		return stage, ladder, raw
	}

	stage, ladder, _ := read("published")
	if stage != "finish" {
		t.Errorf("published stage = %q, want finish", stage)
	}
	wantStages := []string{"probe", "vision", "finish"}
	if len(ladder) != len(wantStages) {
		t.Fatalf("published ladder = %v, want %d records", ladder, len(wantStages))
	}
	for i, want := range wantStages {
		if ladder[i]["stage"] != want {
			t.Errorf("ladder[%d].stage = %v, want %s (order must survive)", i, ladder[i]["stage"], want)
		}
	}
	if last := ladder[2]; last["status"] != "done" || last["attempts"] != float64(2) || last["at"] != "2026-11-01T00:00:02Z" {
		t.Errorf("renamed record lost its other fields: %v", last)
	}
	if ladder[1]["note"] != "vision is off" {
		t.Errorf("an untouched record lost its note: %v", ladder[1])
	}

	if stage, _, raw := read("midway"); stage != "transcode" || raw != midway {
		t.Errorf("a row with no score record changed: stage=%q ladder=%s", stage, raw)
	}
	if stage, _, raw := read("empty"); stage != "probe" || raw != "[]" {
		t.Errorf("an empty ladder changed: stage=%q ladder=%s", stage, raw)
	}
}
