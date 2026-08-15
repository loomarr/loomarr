package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestDowngradeGuard: if the DB records a schema version newer than the binary
// embeds, Open must refuse (§16) instead of limping into an unknown schema.
func TestDowngradeGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.db")

	// Open+migrate normally so goose's version table exists and is current.
	s, err := Open(context.Background(), "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	// Forge a from-the-future version row (higher than any embedded migration).
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Re-open: the guard must fire.
	_, err = Open(context.Background(), "sqlite://"+path, true)
	if err == nil {
		t.Fatal("expected downgrade guard to refuse a from-the-future DB")
	}
}

func TestMigrationProviderIsDialectLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}

	// Changing the legacy process-global dialect must not affect this provider.
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatalf("migrate SQLite after changing legacy dialect: %v", err)
	}

	got, err := provider.GetDBVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}

func TestMigrationProviderScopesEmbeddedMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	if len(sources) == 0 {
		t.Fatal("provider has no migration sources")
	}

	want, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := sources[len(sources)-1].Version; got != want {
		t.Fatalf("highest provider source = %d, want %d", got, want)
	}
}

func TestMigrationProviderRejectsUnknownDialect(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "unknown.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := newMigrationProvider(db, Dialect("unknown"), "migrations/sqlite"); err == nil {
		t.Fatal("expected an unknown dialect error")
	}
}
