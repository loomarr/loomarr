package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// PreMigrationSnapshotsKept is how many pre-migration snapshots stay beside the database.
const PreMigrationSnapshotsKept = 3

// preMigrationDir is where an upgrade keeps the database as it was: beside the database, on the
// same disk, so the copy is quick and needs no setting that is only readable once migrated (#4383).
func preMigrationDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups", "pre-migration")
}

// preMigrationName matches exactly the names snapshotBeforeMigrate writes. Pruning reads only
// these, so nothing else in the directory is ever removed.
var preMigrationName = regexp.MustCompile(`^loomarr-pre-\d+-to-\d+-\d{8}T\d{6}Z\.db$`)

// snapshotBeforeMigrate keeps a checked copy of the SQLite database at dbPath before migrations
// move it from schema `from` to `to` (#4383). Going back is putting the copy in the database's
// place and starting the previous release, which the downgrade guard otherwise refuses.
//
// Any failure here is returned, and the caller does not migrate: an upgrade with no way back is
// worse than an upgrade that waits for disk space.
func snapshotBeforeMigrate(ctx context.Context, db *sql.DB, dbPath string, from, to int64, now time.Time) (string, error) {
	dir := preMigrationDir(dbPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("pre-migration snapshot: create %s: %w", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("loomarr-pre-%d-to-%d-%s.db", from, to, now.UTC().Format("20060102T150405Z")))
	// Nothing else uses the database yet, so the store's own handle is free for the copy.
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("pre-migration snapshot: copy to %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("pre-migration snapshot: chmod %s: %w", path, err)
	}
	if err := quickCheck(ctx, path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("pre-migration snapshot: %s: %w", path, err)
	}
	return path, nil
}

// quickCheck opens the file at path on its own and runs SQLite's quick_check on it.
func quickCheck(ctx context.Context, path string) error {
	check, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", sqliteURIPath(path)))
	if err != nil {
		return fmt.Errorf("open for check: %w", err)
	}
	defer func() { _ = check.Close() }()
	var result string
	if err := check.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("quick_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("quick_check: %s", result)
	}
	return nil
}

// prunePreMigrationSnapshots keeps the newest `keep` snapshots in dir and removes the rest. It
// runs after a snapshot is written, never before, so a failed copy never costs an older one.
func prunePreMigrationSnapshots(dir string, keep int) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var ours []string
	for _, e := range entries {
		if !e.IsDir() && preMigrationName.MatchString(e.Name()) {
			ours = append(ours, e.Name())
		}
	}
	// Newest first, by the time in the name; the schema numbers before it only ever grow.
	stamp := func(name string) string { return name[len(name)-len("20060102T150405Z.db"):] }
	sort.Slice(ours, func(i, j int) bool { return stamp(ours[i]) > stamp(ours[j]) })
	removed := 0
	for _, name := range ours[min(keep, len(ours)):] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
