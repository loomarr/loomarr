package store

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

// Embedded migration FS (§5): separate per-dialect dirs so dialect DDL never
// leaks across backends. goose selects the dir matching the active driver.
//
//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationFS embed.FS

// SQLiteMigrationDigest names the SQLite schema this binary migrates to: a SHA-256 over every
// embedded SQLite migration's file name and bytes, in name order. Applied migrations are immutable
// (§16), so two binaries with the same digest build byte-identical schemas; testkit keys its
// cross-process migrated template on it.
func SQLiteMigrationDigest() (string, error) {
	const dir = "migrations/sqlite"
	entries, err := fs.ReadDir(migrationFS, dir) // sorted by name
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, e := range entries {
		body, err := migrationFS.ReadFile(dir + "/" + e.Name())
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(e.Name()))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(body)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
