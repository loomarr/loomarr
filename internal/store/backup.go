package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// StreamBackup writes a consistent SQLite snapshot to w via VACUUM INTO a temp
// file (§16) — pure SQL, so it works with the cgo-free driver and is safe while
// WAL is active (never cp a live SQLite file). Only the SQLite backend
// implements this; the Postgres store does not (the API returns 501 there).
func (s *sqlStore) StreamBackup(ctx context.Context, w io.Writer) error {
	if s.dialect != DialectSQLite {
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

// BackupFile describes a backup written to disk.
type BackupFile struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	WrittenAt int64  `json:"writtenAt"` // Unix seconds, per the schema's epoch convention
}

// WriteBackup writes a snapshot into dir and returns what it wrote (§16, V11).
//
// ⚠ This exists because the migration's backup gate has to be enforceable by the SERVER.
// `StreamBackup` sends a file to the browser and keeps nothing, so the only evidence a
// backup happened would be the client's word for it — and "a backup is required, not
// suggested" is not a requirement if the thing being gated can assert its own compliance.
// A file on disk with a timestamp is checkable.
//
// Same VACUUM INTO snapshot as the streaming path; the difference is that the file is
// kept, in `backup.dir`, rather than copied to a writer and deleted.
func (s *sqlStore) WriteBackup(ctx context.Context, dir string) (BackupFile, error) {
	if s.dialect != DialectSQLite {
		// Postgres has mature, well-understood backup tooling and an operator running it
		// already has a strategy; inventing a second one here would be worse than theirs.
		return BackupFile{}, fmt.Errorf("WriteBackup: not a SQLite store")
	}
	if dir == "" {
		return BackupFile{}, fmt.Errorf("WriteBackup: no backup directory configured")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return BackupFile{}, fmt.Errorf("create %s: %w", dir, err)
	}

	now := time.Now()
	// Sortable, second-resolution, no colons — the filename is read by humans in a
	// listing and typed into a shell, and a colon is awkward in both.
	name := "loomarr-" + now.UTC().Format("2006-01-02-150405") + ".db"
	path := filepath.Join(dir, name)

	// VACUUM INTO refuses to overwrite, which is the behaviour we want: a same-second
	// collision should surface, not silently replace the earlier backup.
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return BackupFile{}, fmt.Errorf("vacuum into %s: %w", path, err)
	}
	// 0600: a backup carries every secret the instance holds (§16) — the generated
	// playout token, the API token, stored provider keys.
	if err := os.Chmod(path, 0o600); err != nil {
		return BackupFile{}, fmt.Errorf("chmod %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return BackupFile{}, fmt.Errorf("stat %s: %w", path, err)
	}
	return BackupFile{Path: path, Bytes: info.Size(), WrittenAt: now.Unix()}, nil
}

// SQLiteBackuper exposes StreamBackup when the store is the SQLite backend; the
// API uses it to decide SQLite-stream vs. Postgres-501 (§16). Returns nil for a
// non-SQLite store.
func SQLiteBackuper(st Store) interface {
	StreamBackup(ctx context.Context, w io.Writer) error
} {
	s, ok := st.(*sqlStore)
	if !ok || s.dialect != DialectSQLite {
		return nil
	}
	return s
}

// BackupWriter exposes WriteBackup for a SQLite store, nil otherwise. Separate from
// SQLiteBackuper rather than widening it: that interface is the streaming download's
// capability probe and is wired through the composition root, and a nil-vs-non-nil
// capability check should stay about one capability.
func BackupWriter(st Store) interface {
	WriteBackup(ctx context.Context, dir string) (BackupFile, error)
} {
	s, ok := st.(*sqlStore)
	if !ok || s.dialect != DialectSQLite {
		return nil
	}
	return s
}

// DialectOf reports a store's backend, or "" if it is not a SQL store. The Database
// settings page needs it to know whether to offer a migration at all.
func DialectOf(st Store) Dialect {
	if s, ok := st.(*sqlStore); ok {
		return s.dialect
	}
	return ""
}
