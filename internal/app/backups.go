package app

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/scheduler"
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

// --- the scheduled backup (§16, §18.1) ---

// backupJobName, backupJobTitle and backupJobCron are shared by BOTH branches below.
//
// ⚠ They are constants rather than repeated literals because the two branches must agree:
// the Tasks page keys off the NAME, so a SQLite install and a Postgres install disagreeing
// on it would look like two different jobs to anyone comparing them, and a settings override
// written on one would not apply to the other.
const (
	backupJobName  = "backup"
	backupJobTitle = "Back up the database"
	// One description for both the runnable and the disabled listing: the job DOES the same
	// thing either way, and the reason it cannot run here is its DisabledReason, not its blurb.
	backupJobDesc = "Writes a snapshot of the database to your backup folder, then deletes the oldest ones past the number you keep."
	backupJobCron = "0 30 3 * * *"
	backupJobKey  = "backup.schedule"
)

// Job returns the scheduled backup.
//
// ⚠ It calls the SAME service the "Back up now" button does, rather than a second copy of
// write-then-prune. Two implementations of a retention policy is how the scheduled path and
// the manual path come to disagree about which files are safe to delete.
func (s *backupsService) Job(log *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name: backupJobName, Group: scheduler.GroupBackup, Title: backupJobTitle, Description: backupJobDesc,
		DefaultCron: backupJobCron, ScheduleKey: backupJobKey,
		Run: func(ctx context.Context) error {
			entry, err := s.Run(ctx)
			if err != nil {
				return err
			}
			if log != nil {
				log.Info("backup written", "name", entry.Name, "bytes", entry.Bytes)
			}
			return nil
		},
	}
}

// unavailableBackupJob is the Postgres listing: the job EXISTS and says why it cannot run.
//
// ⚠ Registering a disabled row rather than nothing at all is the point. An absent row is
// indistinguishable, from the Tasks page alone, from a job that runs fine and has simply
// never failed — and for backup that ambiguity means an operator believing they are covered
// when they are not.
//
// No Run func: the scheduler never calls one for a disabled job, so leaving it nil means a
// regression that DID schedule it panics loudly in tests rather than silently running a
// no-op that looks like a successful backup.
func unavailableBackupJob(reason string) scheduler.Job {
	return scheduler.Job{
		Name: backupJobName, Group: scheduler.GroupBackup, Title: backupJobTitle, Description: backupJobDesc,
		DefaultCron: backupJobCron, ScheduleKey: backupJobKey,
		DisabledReason: reason,
	}
}
