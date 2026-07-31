package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// backupNamePattern matches exactly the filenames WriteBackup produces:
// loomarr-YYYY-MM-DD-HHMMSS.db. It is the safety boundary for BOTH listing and
// pruning (§16, V12).
//
// ⚠ `backup.dir` is operator-configurable and may point at a directory holding other
// things — a NAS share, a folder that also receives another tool's dumps. Retention
// that deleted "the oldest files in the directory" would delete files this code never
// created, which is data loss wearing a feature's clothes. So both readers filter on
// this pattern and anything else in the directory is invisible to them.
var backupNamePattern = regexp.MustCompile(`^loomarr-\d{4}-\d{2}-\d{2}-\d{6}\.db$`)

// IsBackupName reports whether name is one of ours. Exported because the download
// handler validates a CLIENT-SUPPLIED path segment against the same rule, and two
// definitions of "a backup filename" is one more than can stay in agreement.
func IsBackupName(name string) bool { return backupNamePattern.MatchString(name) }

// ListBackups returns the backups in dir, NEWEST FIRST.
//
// Sorted by filename, which is equivalent to sorting by time because the timestamp
// format is fixed-width and lexically sortable — deliberately chosen for that in
// WriteBackup. Reading mtime instead would be a second source of truth that a file
// copy or a restore could desynchronize from the name the operator reads.
//
// A missing or unreadable directory is an EMPTY LIST, not an error: "no backup has
// been written yet" is the normal state of a fresh install, and a 5xx on the Backup
// page would read as a broken feature rather than an empty one.
func ListBackups(dir string) ([]BackupFile, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	out := make([]BackupFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !IsBackupName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			// A file that vanished between ReadDir and Info (a concurrent prune) is
			// not an error for the caller — it is simply no longer a backup.
			continue
		}
		out = append(out, BackupFile{
			Path:      filepath.Join(dir, e.Name()),
			Bytes:     info.Size(),
			WrittenAt: info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path > out[j].Path })
	return out, nil
}

// PruneBackups deletes all but the newest `retain` backups in dir and returns how many
// it removed (§16, V12).
//
// retain <= 0 keeps everything. That is the honest reading of "keep 0 backups": an
// operator who wants unbounded history needs a way to say so, and the alternative
// meaning — delete every backup including the one just written — is never what anyone
// wants from a retention setting.
//
// ⚠ Callers must prune AFTER a successful write, never before. Pruning first would
// satisfy retention by destroying the oldest backup and then, if the snapshot fails,
// leave the instance with fewer backups than it started with — the worst possible
// behaviour at the exact moment the database is unhealthy.
func PruneBackups(dir string, retain int) (int, error) {
	if retain <= 0 {
		return 0, nil
	}
	files, err := ListBackups(dir) // newest first
	if err != nil {
		return 0, err
	}
	if len(files) <= retain {
		return 0, nil
	}
	removed := 0
	var firstErr error
	for _, f := range files[retain:] {
		if err := os.Remove(f.Path); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("prune %s: %w", f.Path, err)
			}
			continue
		}
		removed++
	}
	return removed, firstErr
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

// SQLitePath returns the file a SQLite store is backed by, or "" for any other backend.
//
// Exists so a caller can locate the data directory (where bootstrap.json lives) without
// being handed the DSN — BuildHandler deliberately takes a Store rather than the config,
// and threading DATABASE_URL through it just to re-derive a path the store already knows
// would widen that signature for no gain.
func SQLitePath(st Store) string {
	s, ok := st.(*sqlStore)
	if !ok || s.dialect != DialectSQLite {
		return ""
	}
	return s.path
}

// DialectOf reports a store's backend, or "" if it is not a SQL store. The Database
// settings page needs it to know whether to offer a migration at all.
func DialectOf(st Store) Dialect {
	if s, ok := st.(*sqlStore); ok {
		return s.dialect
	}
	return ""
}

// PoolOf returns the underlying *sql.DB, or nil for a non-SQL store. Same shape as DialectOf
// above, and used for the same kind of reason: the background-job engine (§18.1) is built on
// River, which takes a database/sql pool directly.
//
// ⚠ **Deliberately NOT on the Store interface.** Store is the port every consumer depends on,
// and handing out the raw pool there would let any of them bypass the store's own methods and
// write SQL against the catalog — the boundary §2 exists to keep. The composition root is the
// one place that legitimately needs it, and a type assertion keeps that visible: a caller
// reaching for this is doing something unusual and should look like it.
//
// ⚠ River SHARES this pool rather than opening its own. On SQLite that is load-bearing, not a
// convenience: the store sets MaxOpenConns(1) because modernc's driver serializes writes, and a
// second pool against the same file would reintroduce exactly the WAL write contention that
// setting exists to avoid.
func PoolOf(st Store) *sql.DB {
	if s, ok := st.(*sqlStore); ok {
		return s.db
	}
	return nil
}
