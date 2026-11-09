// Package databasemigration is the SQLite→PostgreSQL migration stepper's backend (§18, V11): it
// preflights a target, takes the backup the migration requires, and admits one atomic
// migrate-and-restart request.
package databasemigration

import (
	"context"
	"errors"
	"sync"

	"github.com/loomarr/loomarr/internal/platform/config"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Sentinels Migrate and Preflight return; the HTTP surface maps them to status codes.
var (
	// ErrNoBackup: Migrate was called before a backup was taken.
	ErrNoBackup = errors.New("a backup is required before migrating")
	// ErrPreflightFailed: Migrate was called with a target that has not passed preflight.
	ErrPreflightFailed = errors.New("target has not passed preflight")
	// ErrNotSQLite: the install is already on Postgres, so there is nothing to migrate.
	ErrNotSQLite = errors.New("this install is not on SQLite")
	// ErrMigrationRunning: a migration is already in flight.
	ErrMigrationRunning = errors.New("a migration is already running")
	// ErrDatabaseURLPinned: an environment pin would override the migrated target on restart.
	ErrDatabaseURLPinned = errors.New("DATABASE_URL is pinned by the environment")
	// ErrMigrationUnavailable: no process-level migration callback is attached.
	ErrMigrationUnavailable = errors.New("atomic database migration is unavailable")
)

// Check is one preflight check's outcome.
type Check struct {
	Name   string
	Detail string
	OK     bool
}

// Table is one table's row counts — the unit parity is asserted in.
type Table struct {
	Table  string
	Source int64
	Copied int64
}

// Backup describes a server-written backup file.
type Backup struct {
	Path      string
	Bytes     int64
	WrittenAt int64 // Unix seconds
}

// Status is the stepper's whole view of the world.
type Status struct {
	Backend string // sqlite or postgres
	// CanMigrate is false once the install is already on Postgres.
	CanMigrate bool
	Phase      string  // idle|migrating|verified|failed
	Tables     []Table // per-table progress; empty when idle
	// Parity is the GATE, reported apart from Tables so a caller need not compare every row.
	Parity string // unknown|match|mismatch
	Backup *Backup
	Error  string
}

// Events announces admission-state changes.
type Events interface {
	Changed(Status)
}

// Service is the migration stepper's backend.
//
// ⚠ **This type is where the backup gate is actually enforced.** The mock disables the
// Migrate button until a backup exists; that is a hint. The requirement is that a caller
// who ignores the UI entirely and POSTs straight to /migrate is refused — so the state
// the gate reads (`backup`, `preflighted`) lives here on the server, is set only by this
// server's own Backup/Preflight calls, and is never accepted from the request.
//
// State is in-memory and per-generation on purpose. This module admits a request; the
// process owner performs the copy only after draining and closing the live store. A
// failed atomic attempt starts a fresh SQLite generation and injects its error through
// WithLastError, while a successful attempt starts on PostgreSQL.
type Service struct {
	dialect store.Dialect
	writer  BackupWriter  // nil unless the source is SQLite
	backupD func() string // reads backup.dir from settings at call time (hot-applied)
	events  Events
	// requestMigration must enqueue and return promptly. It is the seam to the process
	// generation owner; this module never starts a copy goroutine of its own.
	requestMigration func(dsn string) error

	mu sync.Mutex
	// preflighted is the DSN that most recently passed every check. Compared by exact
	// string, so editing any connection field invalidates it — which is intended: a
	// preflight result is about one target, not about the operator's good intentions.
	preflighted string
	backup      *Backup
	phase       string // idle|migrating|verified|failed
	tables      []Table
	parity      string // unknown|match|mismatch
	lastErr     string
	running     bool
}

// BackupWriter writes a server-side snapshot of the source database.
type BackupWriter interface {
	WriteBackup(ctx context.Context, dir string) (store.BackupFile, error)
}

// New returns the service over a source database of the given dialect. writer is nil unless the
// source is SQLite. backupDir resolves backup.dir at call time; events may be nil.
func New(dialect store.Dialect, writer BackupWriter, backupDir func() string, events Events) *Service {
	return &Service{dialect: dialect, writer: writer, backupD: backupDir, events: events, phase: "idle", parity: "unknown"}
}

// WithMigrationRequest attaches the process-level atomic migration requester. The
// callback owns drain/copy/verify/bootstrap/restart and MUST only enqueue here.
func (d *Service) WithMigrationRequest(request func(string) error) *Service {
	d.requestMigration = request
	return d
}

// WithLastError carries a failed attempt across the generation restart. The source
// remained SQLite, so the new generation can report the failure without durable
// migration state authorizing a stale retry.
func (d *Service) WithLastError(lastErr string) *Service {
	if lastErr == "" {
		return d
	}
	d.mu.Lock()
	d.phase = "failed"
	d.lastErr = lastErr
	d.mu.Unlock()
	return d
}

func (d *Service) Status(context.Context) (Status, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusLocked(), nil
}

func (d *Service) statusLocked() Status {
	backend := string(d.dialect)
	return Status{
		Backend: backend,
		// Only SQLite installs are offered a migration: Postgres→SQLite is not a
		// direction anyone migrates, and offering a disabled control would raise the
		// question rather than answer it.
		CanMigrate: store.CanMigrateToPostgres(d.dialect),
		Phase:      d.phase,
		Tables:     d.tables,
		Parity:     d.parity,
		Backup:     d.backup,
		Error:      d.lastErr,
	}
}

func (d *Service) Preflight(ctx context.Context, dsn string) ([]Check, error) {
	if !store.CanMigrateToPostgres(d.dialect) {
		return nil, ErrNotSQLite
	}
	checks, err := store.Preflight(ctx, dsn)
	if err != nil {
		return nil, err
	}
	out := make([]Check, 0, len(checks))
	for _, c := range checks {
		out = append(out, Check{Name: c.Name, Detail: c.Detail, OK: c.OK})
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if store.PreflightPassed(checks) {
		d.preflighted = dsn
	} else if d.preflighted == dsn {
		// A target that USED to pass and now doesn't must lose its authorization —
		// otherwise a stale pass would carry a migration into a target that has since
		// had tables created in it.
		d.preflighted = ""
	}
	return out, nil
}

func (d *Service) Backup(ctx context.Context) (Backup, error) {
	w := d.writer
	if w == nil {
		return Backup{}, ErrNotSQLite
	}
	bk, err := w.WriteBackup(ctx, d.backupD())
	if err != nil {
		return Backup{}, err
	}
	out := Backup{Path: bk.Path, Bytes: bk.Bytes, WrittenAt: bk.WrittenAt}

	d.mu.Lock()
	d.backup = &out
	d.mu.Unlock()
	return out, nil
}

func (d *Service) Migrate(_ context.Context, dsn string) error {
	// The complete gate runs before the process-level request is queued.
	d.mu.Lock()
	switch {
	case !store.CanMigrateToPostgres(d.dialect):
		d.mu.Unlock()
		return ErrNotSQLite
	case d.running:
		d.mu.Unlock()
		return ErrMigrationRunning
	case config.PinnedByEnv("DATABASE_URL"):
		d.mu.Unlock()
		return ErrDatabaseURLPinned
	case d.preflighted != dsn:
		d.mu.Unlock()
		return ErrPreflightFailed
	case d.backup == nil:
		d.mu.Unlock()
		return ErrNoBackup
	case d.requestMigration == nil:
		d.mu.Unlock()
		return ErrMigrationUnavailable
	}
	d.running = true
	d.phase = "migrating"
	d.parity = "unknown"
	d.lastErr = ""
	d.tables = nil
	st := d.statusLocked()
	d.mu.Unlock()
	d.emit(st)

	// Synchronous only through enqueue. The callback's interface requires it to return
	// before drain begins so the HTTP response can reach the operator.
	if err := d.requestMigration(dsn); err != nil {
		d.mu.Lock()
		d.running = false
		d.phase = "failed"
		d.lastErr = err.Error()
		st = d.statusLocked()
		d.mu.Unlock()
		d.emit(st)
		return err
	}
	return nil
}

// emit announces admission-state changes. It does not claim copy progress: once the
// request is accepted the generation drains, and the reconnecting status endpoint is
// the source of truth for either the new PostgreSQL generation or a carried failure.
func (d *Service) emit(st Status) {
	if d.events == nil {
		return
	}
	d.events.Changed(st)
}
