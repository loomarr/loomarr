package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// StreamBackup writes a consistent SQLite snapshot to w via VACUUM INTO a temp
// file (§16) — pure SQL, so it works with the cgo-free driver and is safe while
// WAL is active (never cp a live SQLite file). Only the SQLite backend
// implements this; the Postgres store does not (the API returns 501 there).
func (s *sqlStore) StreamBackup(ctx context.Context, w io.Writer) error {
	if s.claimSQL != sqliteClaimSQL {
		return fmt.Errorf("StreamBackup: not a SQLite store")
	}
	tmp, err := os.CreateTemp("", "loomarr-backup-*.db")
	if err != nil {
		return fmt.Errorf("backup temp: %w", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	// VACUUM INTO produces a consistent snapshot file.
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("stream snapshot: %w", err)
	}
	return nil
}

// SQLiteBackuper exposes StreamBackup when the store is the SQLite backend; the
// API uses it to decide SQLite-stream vs. Postgres-501 (§16). Returns nil for a
// non-SQLite store.
func SQLiteBackuper(st Store) interface {
	StreamBackup(ctx context.Context, w io.Writer) error
} {
	s, ok := st.(*sqlStore)
	if !ok || s.claimSQL != sqliteClaimSQL {
		return nil
	}
	return s
}
