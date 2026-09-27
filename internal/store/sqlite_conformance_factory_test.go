package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestSQLiteStoreCloneMatchesFreshReplay is the drift guard for newSQLiteStore's shared template
// (#1570): a clone must be indistinguishable from replaying every migration and boot seed, in
// schema, applied migration versions, and seeded rows. It cannot go stale — the template is built
// by the same Open(..., true) this test compares against, once per test process.
func TestSQLiteStoreCloneMatchesFreshReplay(t *testing.T) {
	ctx := context.Background()
	replayed, err := Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "replayed.db"), true)
	if err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	t.Cleanup(func() { _ = replayed.Close() })

	first := newSQLiteStore(t)
	second := newSQLiteStore(t)
	if builds := sqliteTemplateBuilds.Load(); builds != 1 {
		t.Fatalf("migrated sqlite template built %d times, want once per test process", builds)
	}

	want := sqliteFingerprint(t, replayed.(*sqlStore).db)
	for name, clone := range map[string]Store{"first": first, "second": second} {
		if got := sqliteFingerprint(t, clone.(*sqlStore).db); got != want {
			t.Fatalf("%s clone drifted from a fresh replay\n got: %s\nwant: %s", name, got, want)
		}
	}
}

// sqliteFingerprint renders what a migrated, boot-seeded database consists of: every schema
// object, every goose version row, and each table's row count.
func sqliteFingerprint(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	var b strings.Builder
	rows, err := db.QueryContext(ctx, `SELECT type, name, COALESCE(sql, '') FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatalf("read sqlite schema: %v", err)
	}
	var tables []string
	for rows.Next() {
		var kind, name, ddl string
		if err := rows.Scan(&kind, &name, &ddl); err != nil {
			t.Fatalf("scan sqlite schema: %v", err)
		}
		fmt.Fprintf(&b, "%s %s: %s\n", kind, name, ddl)
		if kind == "table" {
			tables = append(tables, name)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatalf("read sqlite schema: %v", err)
	}

	versions, err := db.QueryContext(ctx, `SELECT version_id, is_applied FROM goose_db_version ORDER BY id`)
	if err != nil {
		t.Fatalf("read goose versions: %v", err)
	}
	for versions.Next() {
		var version int64
		var applied bool
		if err := versions.Scan(&version, &applied); err != nil {
			t.Fatalf("scan goose versions: %v", err)
		}
		fmt.Fprintf(&b, "version %d applied=%t\n", version, applied)
	}
	if err := errors.Join(versions.Err(), versions.Close()); err != nil {
		t.Fatalf("read goose versions: %v", err)
	}

	for _, table := range tables {
		var count int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+table+`"`).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		fmt.Fprintf(&b, "rows %s=%d\n", table, count)
	}
	return b.String()
}

func TestSQLiteConformanceFactoryClonesMigratedIsolatedStores(t *testing.T) {
	t.Parallel()

	var migrationModes []bool
	newStore := newSQLiteConformanceStoreFactoryWithOpen(t, func(ctx context.Context, dsn string, autoMigrate bool) (Store, error) {
		migrationModes = append(migrationModes, autoMigrate)
		return Open(ctx, dsn, autoMigrate)
	})
	left := newStore(t)
	right := newStore(t)
	ctx := context.Background()
	if len(migrationModes) != 3 || !migrationModes[0] || migrationModes[1] || migrationModes[2] {
		t.Fatalf("auto-migrate modes = %v, want one template migration followed by clone-only opens", migrationModes)
	}

	wantVersion, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := SchemaVersion(left); got != wantVersion {
		t.Fatalf("cloned schema version = %d, want %d", got, wantVersion)
	}
	taxa, err := right.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(taxa) == 0 {
		t.Fatal("cloned store lost boot-seeded taxonomy")
	}

	if err := left.SetSetting(ctx, "clone.isolation", "left-only"); err != nil {
		t.Fatal(err)
	}
	if _, err := right.GetSetting(ctx, "clone.isolation"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second clone observed first clone's write: %v", err)
	}
}
