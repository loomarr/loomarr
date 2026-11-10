package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

// libraryCatalogJSONMigration declares library_catalog.record JSON (#2793).
const libraryCatalogJSONMigration = 216

func TestLibraryCatalogJSONMigration(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "catalog-json.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testLibraryCatalogJSONMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testLibraryCatalogJSONMigration proves the migration keeps every fetched record, turns the empty
// record of an unfetched or unknown title into NULL, and that afterwards a record must be JSON.
func testLibraryCatalogJSONMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, libraryCatalogJSONMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", libraryCatalogJSONMigration-1, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO library_catalog (provider, kind, external_id, record, fetched_at_ms) VALUES
		('tmdb', 'movie', '90000001', '{"Kind":"movie","IDs":[]}', 5),
		('tmdb', 'movie', '90000002', '', 5),
		('tmdb', 'movie', '90000003', '', 0)`); err != nil {
		t.Fatalf("seed before %d: %v", libraryCatalogJSONMigration, err)
	}
	if _, err := provider.UpTo(ctx, libraryCatalogJSONMigration); err != nil {
		t.Fatalf("apply %d: %v", libraryCatalogJSONMigration, err)
	}

	record := func(id string) (string, int64) {
		t.Helper()
		var raw sql.NullString
		var fetched int64
		if err := db.QueryRowContext(ctx, `SELECT record, fetched_at_ms FROM library_catalog WHERE external_id = '`+id+`'`).
			Scan(&raw, &fetched); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
		if !raw.Valid {
			return "NULL", fetched
		}
		return raw.String, fetched
	}
	if raw, _ := record("90000001"); raw == "NULL" || !json.Valid([]byte(raw)) {
		t.Errorf("fetched record = %s, want its JSON kept", raw)
	}
	if raw, fetched := record("90000002"); raw != "NULL" || fetched != 5 {
		t.Errorf("unknown title = %s fetched %d, want NULL and still fetched", raw, fetched)
	}
	if raw, fetched := record("90000003"); raw != "NULL" || fetched != 0 {
		t.Errorf("unfetched title = %s fetched %d, want NULL and never fetched", raw, fetched)
	}
	if _, err := db.ExecContext(ctx, `UPDATE library_catalog SET record = 'not json' WHERE external_id = '90000002'`); err == nil {
		t.Error("stored a record that is not JSON")
	}
}
