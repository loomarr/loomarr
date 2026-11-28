package store

import (
	"context"
	"database/sql"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"os"
	"path/filepath"
	"testing"
)

// olderSchema makes a SQLite database at path whose schema stops one migration short of this
// build's, as an install on the previous release has it. It is a copy of the migration ladder's
// file at that version, opened with the store's own settings.
func olderSchema(t *testing.T, path string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "versions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	p, err := newMigrationProvider(db, storedb.DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	sources := p.ListSources()
	older := sources[len(sources)-2].Version
	file, err := os.ReadFile(sqliteFileAt(t, older, storeConnections))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	return older
}

func schemaVersion(t *testing.T, path string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var v int64
	if err := db.QueryRow("SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// An upgrade keeps a checked copy of the database as it was before the migrations ran, beside it,
// so going back is putting that file back and starting the previous release.
func TestAnUpgradeKeepsTheDatabaseAsItWasBeforeMigrating(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "loomarr.db")
	older := olderSchema(t, path)

	s, err := Open(context.Background(), "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	snaps, err := filepath.Glob(filepath.Join(dir, "backups", "pre-migration", "loomarr-pre-*.db"))
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snapshots %v (err %v), want one", snaps, err)
	}
	if v := schemaVersion(t, snaps[0]); v != older {
		t.Errorf("the snapshot is at schema %d, want the pre-upgrade %d", v, older)
	}
	if v := schemaVersion(t, path); v <= older {
		t.Errorf("the database is at %d, want it migrated past %d", v, older)
	}
	if info, err := os.Stat(snaps[0]); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("snapshot mode %v (err %v), want 0600", info.Mode().Perm(), err)
	}
}

// A new install and a restart with nothing to migrate take no snapshot.
func TestNothingToMigrateTakesNoSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "loomarr.db")
	for range 2 { // a fresh install, then a plain restart
		s, err := Open(context.Background(), "sqlite://"+path, true)
		if err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", "pre-migration")); !os.IsNotExist(err) {
		t.Errorf("a snapshot directory exists (err %v); nothing was migrated from an existing schema", err)
	}
}

// When the copy cannot be made, the migration does not run: the database stays at the version
// the previous release can still open.
func TestAnUpgradeThatCannotKeepACopyDoesNotMigrate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "loomarr.db")
	older := olderSchema(t, path)
	// A file where the snapshot directory should be: nothing can be written there.
	if err := os.WriteFile(filepath.Join(dir, "backups"), []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(context.Background(), "sqlite://"+path, true); err == nil {
		_ = s.Close()
		t.Fatal("the upgrade migrated with no copy of the database kept")
	}
	if v := schemaVersion(t, path); v != older {
		t.Errorf("the database moved to %d, want it left at %d", v, older)
	}
}

// Only the newest few snapshots are kept, and nothing else in the directory is touched.
func TestOldSnapshotsArePrunedAndOtherFilesKept(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	snapDir := filepath.Join(dir, "backups", "pre-migration")
	if err := os.MkdirAll(snapDir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"loomarr-pre-200-to-210-20261101T000000Z.db", "loomarr-pre-210-to-214-20261102T000000Z.db",
		"loomarr-pre-214-to-230-20261103T000000Z.db", "someone-elses.db",
	} {
		if err := os.WriteFile(filepath.Join(snapDir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "loomarr.db")
	olderSchema(t, path)
	s, err := Open(context.Background(), "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	left, _ := filepath.Glob(filepath.Join(snapDir, "*"))
	if len(left) != PreMigrationSnapshotsKept+1 {
		t.Fatalf("left %v, want the newest %d snapshots and the file that isn't ours", left, PreMigrationSnapshotsKept)
	}
	if _, err := os.Stat(filepath.Join(snapDir, "loomarr-pre-200-to-210-20261101T000000Z.db")); !os.IsNotExist(err) {
		t.Error("the oldest snapshot was kept")
	}
	if _, err := os.Stat(filepath.Join(snapDir, "someone-elses.db")); err != nil {
		t.Error("a file that isn't a snapshot was removed")
	}
}
