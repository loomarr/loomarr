package store

import (
	"context"
	"database/sql"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"path/filepath"
	"testing"
)

// deadIDColumnsMigration drops the two id columns nothing reads (#2793).
const deadIDColumnsMigration = 257

func TestDeadIDColumnsMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "dead-ids.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testDeadIDColumnsMigration(t, db, storedb.DialectSQLite, "migrations/sqlite")
}

// testDeadIDColumnsMigration proves the migration keeps every evaluation, operation and clip or day
// budget guard, deletes the run guards, and removes both columns.
func testDeadIDColumnsMigration(t *testing.T, db *sql.DB, dialect storedb.Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, deadIDColumnsMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", deadIDColumnsMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO filler_inference_evaluations (id, cache_key, clip_hash, role, state, requested_provider,
			requested_model, modalities_json, reserved_nano_usd, evidence_version, extractor_version,
			prompt_version, schema_version, taxonomy_version, admission_policy_version, role_policy_version,
			capability_snapshot, created_at, updated_at)
			VALUES ('e1', 'k', 'clip-a', 'r', 'reserved', 'p', 'm', '[]', 40, 'v', 'v', 'v', 'v', 'v', 'v', 'v', 'c', 1, 1)`,
		`INSERT INTO interactive_operations (id, kind, subject, status, started_at, updated_at)
			VALUES ('op1', 'filler_split', 'clip-a', 'success', 1, 1)`,
		`INSERT INTO filler_inference_budget_guards (scope) VALUES ('clip:clip-a'), ('day:2026-01-01'), ('run:r1')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", deadIDColumnsMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, deadIDColumnsMigration); err != nil {
		t.Fatalf("apply %d: %v", deadIDColumnsMigration, err)
	}

	for query, want := range map[string]int{
		`SELECT count(*) FROM filler_inference_evaluations WHERE id = 'e1' AND reserved_nano_usd = 40`: 1,
		`SELECT count(*) FROM interactive_operations WHERE id = 'op1' AND status = 'success'`:          1,
		`SELECT count(*) FROM filler_inference_budget_guards WHERE scope LIKE 'clip:%'`:                1,
		`SELECT count(*) FROM filler_inference_budget_guards WHERE scope LIKE 'day:%'`:                 1,
		`SELECT count(*) FROM filler_inference_budget_guards WHERE scope LIKE 'run:%'`:                 0,
	} {
		var got int
		if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	for _, query := range []string{
		`SELECT run_id FROM filler_inference_evaluations`,
		`SELECT result_id FROM interactive_operations`,
	} {
		var got string
		if err := db.QueryRowContext(ctx, query).Scan(&got); err == nil || err == sql.ErrNoRows {
			t.Errorf("%s still reads a column: %v", query, err)
		}
	}
}
