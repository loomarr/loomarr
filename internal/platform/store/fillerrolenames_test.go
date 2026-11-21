package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

const fillerRoleNamesMigration = 173

func TestFillerRoleNamesMigration_SQLite(t *testing.T) {
	t.Parallel()
	s, err := openSQLite(context.Background(), sqliteFileAt(t, fillerRoleNamesMigration-1, storeConnections))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testFillerRoleNamesMigration(t, s.db, DialectSQLite, "migrations/sqlite")
}

// testFillerRoleNamesMigration seeds the pre-00173 names in raw SQL — a channel's chosen
// filler.kinds and an accepted 'kind' enrichment axis — and checks the migration moves both to
// "role" without touching the values or any neighbour.
func testFillerRoleNamesMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	bind := func(query string) string {
		if dialect == DialectPostgres {
			return storedb.PgPlaceholders(query)
		}
		return query
	}
	if _, err := provider.UpTo(ctx, fillerRoleNamesMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", fillerRoleNamesMigration-1, err)
	}
	policies := map[string]string{
		"chose-roles": `{"audience":"kids","filler":{"audience":"kids","kinds":["commercial","political"],"pinned":["p1"]}}`,
		"no-roles":    `{"filler":{"audience":"family"}}`,
		"no-filler":   `{"ordering":"shuffle"}`,
	}
	number := 1
	for id, policy := range policies {
		if _, err := db.ExecContext(ctx, bind(`INSERT INTO channels (id, name, number, strategy, status, policy_json)
			VALUES (?, ?, ?, 'sequential', 'building', ?)`), id, id, number, policy); err != nil {
			t.Fatal(err)
		}
		number++
	}
	for _, axis := range []string{"kind", "era"} {
		if _, err := db.ExecContext(ctx, bind(`INSERT INTO filler_enrichment_axes
			(clip_hash, axis, state, value_json, evidence_kind, evidence_rank, evidence_reference, confidence,
			 producer, producer_version, observed_at, updated_at)
			VALUES ('clip', ?, 'complete', '{}', 'item_metadata', 100, 'item.title', 90, 'deterministic', 'v1', 1, 1)`),
			axis); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := provider.UpTo(ctx, fillerRoleNamesMigration); err != nil {
		t.Fatalf("migrate %d: %v", fillerRoleNamesMigration, err)
	}

	want := map[string]string{
		"chose-roles": `{"audience":"kids","filler":{"audience":"kids","roles":["commercial","political"],"pinned":["p1"]}}`,
		"no-roles":    policies["no-roles"],
		"no-filler":   policies["no-filler"],
	}
	for id, wantPolicy := range want {
		var got string
		if err := db.QueryRowContext(ctx, bind(`SELECT policy_json FROM channels WHERE id = ?`), id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		var gotValue, wantValue any
		if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
			t.Fatalf("%s policy_json = %q: %v", id, got, err)
		}
		if err := json.Unmarshal([]byte(wantPolicy), &wantValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotValue, wantValue) {
			t.Errorf("%s policy_json = %s, want %s", id, got, wantPolicy)
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT axis FROM filler_enrichment_axes WHERE clip_hash = 'clip' ORDER BY axis`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var axes []string
	for rows.Next() {
		var axis string
		if err := rows.Scan(&axis); err != nil {
			t.Fatal(err)
		}
		axes = append(axes, axis)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(axes, []string{"era", "role"}) {
		t.Fatalf("axes = %v, want [era role]", axes)
	}
}
