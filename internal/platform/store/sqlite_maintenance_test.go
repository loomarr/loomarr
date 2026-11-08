package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A database made before #2446 has auto_vacuum off; the first open converts it, keeping its rows.
func TestOpenSwitchesAnExistingSQLiteFileToIncrementalVacuum(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE keepsake (v TEXT); INSERT INTO keepsake VALUES ('still here')`); err != nil {
		t.Fatal(err)
	}
	if got := autoVacuumOf(t, raw); got != 0 {
		t.Fatalf("precondition: auto_vacuum = %d, want 0", got)
	}
	_ = raw.Close()

	st, err := Open(ctx, "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	db := PoolOf(st)
	if got := autoVacuumOf(t, db); got != sqliteIncrementalVacuum {
		t.Fatalf("auto_vacuum = %d, want %d (incremental)", got, sqliteIncrementalVacuum)
	}
	var v string
	if err := db.QueryRowContext(ctx, `SELECT v FROM keepsake`).Scan(&v); err != nil || v != "still here" {
		t.Fatalf("row after conversion = (%q, %v)", v, err)
	}
}

// Tidy hands every page deleted rows freed back to the disk, so the file shrinks.
func TestTidyReturnsFreedPagesToTheDisk(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "loomarr.db")
	st, err := Open(ctx, "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	db := PoolOf(st)
	if _, err := db.ExecContext(ctx, `CREATE TABLE scratch (v TEXT)`); err != nil {
		t.Fatal(err)
	}
	blob := strings.Repeat("x", 64<<10)
	for range 64 { // 4 MiB of rows
		if _, err := db.ExecContext(ctx, `INSERT INTO scratch VALUES (?)`, blob); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM scratch`); err != nil {
		t.Fatal(err)
	}
	checkpoint(t, db)
	before := fileSize(t, path)

	freed, err := DatabaseTidier(st).Tidy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if freed < 4<<20 {
		t.Fatalf("freed %d bytes, want at least the 4 MiB deleted", freed)
	}
	var free int
	if err := db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&free); err != nil || free != 0 {
		t.Fatalf("freelist_count after tidy = (%d, %v), want 0", free, err)
	}
	checkpoint(t, db)
	if after := fileSize(t, path); after > before-4<<20 {
		t.Fatalf("file %d → %d bytes, want it at least 4 MiB smaller", before, after)
	}
}

func autoVacuumOf(t *testing.T, db *sql.DB) int {
	t.Helper()
	var mode int
	if err := db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	return mode
}

// checkpoint folds the WAL into the main file, so its size reflects the pages it holds.
func checkpoint(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
