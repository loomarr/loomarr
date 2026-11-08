package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexNamesFollowConvention(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "index-names.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testIndexNamesFollowConvention(t, db, DialectSQLite, "migrations/sqlite")
}

// testIndexNamesFollowConvention holds every index the migrations create to storage.md's
// `idx_<table>_<purpose>` (#2323). Indexes a database makes for a primary key or UNIQUE constraint
// carry the engine's own names and are not checked.
func testIndexNamesFollowConvention(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	query := `SELECT name, tbl_name FROM sqlite_master WHERE type = 'index' AND sql IS NOT NULL`
	if dialect == DialectPostgres {
		query = `SELECT i.indexname, i.tablename FROM pg_indexes i
			WHERE i.schemaname = current_schema() AND i.tablename <> 'goose_db_version'
			  AND NOT EXISTS (SELECT 1 FROM pg_constraint c
			                  WHERE c.conindid = (quote_ident(i.schemaname) || '.' || quote_ident(i.indexname))::regclass)`
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	checked := 0
	for rows.Next() {
		var name, table string
		if err := rows.Scan(&name, &table); err != nil {
			t.Fatal(err)
		}
		checked++
		if name != "idx_"+table && !strings.HasPrefix(name, "idx_"+table+"_") {
			t.Errorf("index %s on %s: want idx_%s_<purpose> (storage.md)", name, table, table)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("checked only %d indexes; the catalog query is not seeing the schema", checked)
	}
}
