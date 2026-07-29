package app

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/store"
)

// backupsService backs /v1/system/backups* (§16, V12) — the backups on disk, as opposed
// to `GET /v1/backup`, which generates a fresh snapshot and keeps nothing.
//
// Wired only for a SQLite install, mirroring the scheduled `backup` job: WriteBackup is
// SQLite-only by design, so on Postgres the API reports 501 and the UI explains pg_dump
// rather than showing an empty table that reads as breakage.
//
// `backup.dir` and `backup.retain` are read at CALL time, not captured at boot, so a
// hot-applied settings change takes effect without a restart — the same pattern
// databaseService uses for the same key.
type backupsService struct {
	w      backupWriter
	dir    func() string
	retain func() int
	sched  func() string
}

// backupWriter is the one capability this service needs from the store — the same shape
// store.BackupWriter returns. Declared here rather than reaching for the store's concrete
// type so the service can be tested with a stub that writes a file and nothing else.
type backupWriter interface {
	WriteBackup(ctx context.Context, dir string) (store.BackupFile, error)
}

// errBackupNotFound covers both "not a name we write" and "not on disk". They are one
// answer on purpose: both mean the caller cannot have the file, and distinguishing them
// would confirm which paths exist.
var errBackupNotFound = errors.New("backup not found")

func (b *backupsService) List(context.Context) (api.BackupList, error) {
	dir := b.dir()
	files, err := store.ListBackups(dir)
	if err != nil {
		return api.BackupList{}, err
	}
	out := api.BackupList{
		Supported: true,
		Dir:       dir,
		Schedule:  b.sched(),
		Retain:    b.retain(),
		Backups:   make([]api.BackupEntry, 0, len(files)),
	}
	for _, f := range files {
		out.Backups = append(out.Backups, api.BackupEntry{
			Name:      filepath.Base(f.Path),
			Bytes:     f.Bytes,
			WrittenAt: f.WrittenAt,
		})
	}
	return out, nil
}

func (b *backupsService) Open(_ context.Context, name string) (api.BackupContent, error) {
	// ⚠ `name` is a client-supplied path segment. Validate it against the pattern the
	// writer produces BEFORE it reaches the filesystem: the pattern admits no separators
	// and no dots beyond the extension, so "../../etc/passwd" is rejected here rather
	// than being cleaned into something that happens to be safe.
	if !store.IsBackupName(name) {
		return api.BackupContent{}, errBackupNotFound
	}
	// Listing rather than stat-ing the joined path keeps ONE definition of "a backup
	// that exists" — the same function the UI's table was built from. A file that is on
	// disk but not in the listing is not something this endpoint should serve.
	files, err := store.ListBackups(b.dir())
	if err != nil {
		return api.BackupContent{}, err
	}
	for _, f := range files {
		if filepath.Base(f.Path) == name {
			return api.BackupContent{Name: name, Path: f.Path, Bytes: f.Bytes}, nil
		}
	}
	return api.BackupContent{}, errBackupNotFound
}

func (b *backupsService) Run(ctx context.Context) (api.BackupEntry, error) {
	dir := b.dir()
	bf, err := b.w.WriteBackup(ctx, dir)
	if err != nil {
		return api.BackupEntry{}, err
	}
	// Prune AFTER the write, never before — see store.PruneBackups. A prune failure does
	// not fail the call: the backup the operator asked for is on disk and valid.
	_, _ = store.PruneBackups(dir, b.retain())
	return api.BackupEntry{
		Name:      filepath.Base(bf.Path),
		Bytes:     bf.Bytes,
		WrittenAt: bf.WrittenAt,
	}, nil
}
