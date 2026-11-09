// Package backup keeps the SQLite backups on disk: it lists them, opens one for download, writes
// a new one and prunes to the retention policy, and supplies the scheduled `backup` job (§16,
// §18.1).
package backup

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Entry is one backup file on disk.
type Entry struct {
	Name      string
	Bytes     int64
	WrittenAt int64 // Unix seconds
}

// List is the backups on disk, newest first, plus the policy in force.
type List struct {
	// Supported is false on Postgres, where in-app backup is deliberately not offered (§16).
	Supported bool
	Dir       string
	Schedule  string
	Retain    int // how many are kept; 0 keeps everything
	Backups   []Entry
}

// Content is one backup opened for download.
type Content struct {
	Name  string
	Path  string
	Bytes int64
}

// Service is the backups on disk, as opposed to `GET /v1/backup`, which generates a fresh
// snapshot and keeps nothing (§16, V12).
//
// It is wired only for a SQLite install, mirroring the scheduled `backup` job: WriteBackup is
// SQLite-only by design, so on Postgres the API reports 501 and the UI explains pg_dump
// rather than showing an empty table that reads as breakage.
//
// `backup.dir` and `backup.retain` are read at CALL time, not captured at boot, so a
// hot-applied settings change takes effect without a restart.
type Service struct {
	w      Writer
	dir    func() string
	retain func() int
	sched  func() string
}

// Config is what a Service reads. Dir, Retain and Schedule resolve at call time.
type Config struct {
	Writer   Writer
	Dir      func() string
	Retain   func() int
	Schedule func() string
}

// New returns the backups service over c.
func New(c Config) *Service {
	return &Service{w: c.Writer, dir: c.Dir, retain: c.Retain, sched: c.Schedule}
}

// Writer is the one capability this service needs from the store — the same shape
// store.BackupWriter returns. Declared here rather than reaching for the store's concrete
// type so the service can be tested with a stub that writes a file and nothing else.
type Writer interface {
	WriteBackup(ctx context.Context, dir string) (store.BackupFile, error)
}

// ErrNotFound covers both "not a name we write" and "not on disk". They are one
// answer on purpose: both mean the caller cannot have the file, and distinguishing them
// would confirm which paths exist.
var ErrNotFound = errors.New("backup not found")

func (b *Service) List(context.Context) (List, error) {
	dir := b.dir()
	files, err := store.ListBackups(dir)
	if err != nil {
		return List{}, err
	}
	out := List{
		Supported: true,
		Dir:       dir,
		Schedule:  b.sched(),
		Retain:    b.retain(),
		Backups:   make([]Entry, 0, len(files)),
	}
	for _, f := range files {
		out.Backups = append(out.Backups, Entry{
			Name:      filepath.Base(f.Path),
			Bytes:     f.Bytes,
			WrittenAt: f.WrittenAt,
		})
	}
	return out, nil
}

func (b *Service) Open(_ context.Context, name string) (Content, error) {
	// ⚠ `name` is a client-supplied path segment. Validate it against the pattern the
	// writer produces BEFORE it reaches the filesystem: the pattern admits no separators
	// and no dots beyond the extension, so "../../etc/passwd" is rejected here rather
	// than being cleaned into something that happens to be safe.
	if !store.IsBackupName(name) {
		return Content{}, ErrNotFound
	}
	// Listing rather than stat-ing the joined path keeps ONE definition of "a backup
	// that exists" — the same function the UI's table was built from. A file that is on
	// disk but not in the listing is not something this endpoint should serve.
	files, err := store.ListBackups(b.dir())
	if err != nil {
		return Content{}, err
	}
	for _, f := range files {
		if filepath.Base(f.Path) == name {
			return Content{Name: name, Path: f.Path, Bytes: f.Bytes}, nil
		}
	}
	return Content{}, ErrNotFound
}

func (b *Service) Run(ctx context.Context) (Entry, error) {
	dir := b.dir()
	bf, err := b.w.WriteBackup(ctx, dir)
	if err != nil {
		return Entry{}, err
	}
	// Prune AFTER the write, never before — see store.PruneBackups. A prune failure does
	// not fail the call: the backup the operator asked for is on disk and valid.
	_, _ = store.PruneBackups(dir, b.retain())
	return Entry{
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
)

// Job returns the scheduled backup.
//
// ⚠ It calls the SAME service the "Back up now" button does, rather than a second copy of
// write-then-prune. Two implementations of a retention policy is how the scheduled path and
// the manual path come to disagree about which files are safe to delete.
func (s *Service) Job(log *slog.Logger) jobs.Job {
	return jobs.Job{
		Name: backupJobName, Group: jobs.GroupBackup, Title: backupJobTitle, Description: backupJobDesc,
		DefaultCron: backupJobCron, ScheduleKey: settings.BackupSchedule.Key(),
		// ⚠ Must be declared: with none the job runs under River's inherited one-minute
		// JobTimeout, and VACUUM INTO on the 1.8 GB household DB took ~57s (#1411).
		Timeout: 15 * time.Minute,
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
func UnavailableJob(reason string) jobs.Job {
	return jobs.Job{
		Name: backupJobName, Group: jobs.GroupBackup, Title: backupJobTitle, Description: backupJobDesc,
		DefaultCron: backupJobCron, ScheduleKey: settings.BackupSchedule.Key(),
		DisabledReason: reason,
	}
}
