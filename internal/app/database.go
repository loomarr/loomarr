package app

import (
	"context"
	"sync"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/store"
)

// databaseService implements api.DatabaseService — the SQLite→PostgreSQL migration
// stepper's backend (§18, V11).
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
type databaseService struct {
	src     store.Store
	dataDir string
	backupD func() string // reads backup.dir from settings at call time (hot-applied)
	bus     *events.Bus
	// requestMigration must enqueue and return promptly. It is the seam to the process
	// generation owner; this module never starts a copy goroutine of its own.
	requestMigration func(dsn string) error

	mu sync.Mutex
	// preflighted is the DSN that most recently passed every check. Compared by exact
	// string, so editing any connection field invalidates it — which is intended: a
	// preflight result is about one target, not about the operator's good intentions.
	preflighted string
	backup      *api.DatabaseBackup
	phase       string // idle|migrating|verified|failed
	tables      []api.DatabaseTable
	parity      string // unknown|match|mismatch
	lastErr     string
	running     bool
}

func newDatabaseService(src store.Store, dataDir string, backupDir func() string, bus *events.Bus) *databaseService {
	return &databaseService{src: src, dataDir: dataDir, backupD: backupDir, bus: bus, phase: "idle", parity: "unknown"}
}

// WithMigrationRequest attaches the process-level atomic migration requester. The
// callback owns drain/copy/verify/bootstrap/restart and MUST only enqueue here.
func (d *databaseService) WithMigrationRequest(request func(string) error) *databaseService {
	d.requestMigration = request
	return d
}

// WithLastError carries a failed attempt across the generation restart. The source
// remained SQLite, so the new generation can report the failure without durable
// migration state authorizing a stale retry.
func (d *databaseService) WithLastError(lastErr string) *databaseService {
	if lastErr == "" {
		return d
	}
	d.mu.Lock()
	d.phase = "failed"
	d.lastErr = lastErr
	d.mu.Unlock()
	return d
}

func (d *databaseService) Status(context.Context) (api.DatabaseStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusLocked(), nil
}

func (d *databaseService) statusLocked() api.DatabaseStatus {
	backend := string(store.DialectOf(d.src))
	return api.DatabaseStatus{
		Backend: backend,
		// Only SQLite installs are offered a migration: Postgres→SQLite is not a
		// direction anyone migrates, and offering a disabled control would raise the
		// question rather than answer it.
		CanMigrate: backend == string(store.DialectSQLite),
		Phase:      d.phase,
		Tables:     d.tables,
		Parity:     d.parity,
		Backup:     d.backup,
		Error:      d.lastErr,
	}
}

func (d *databaseService) Preflight(ctx context.Context, dsn string) ([]api.DatabaseCheck, error) {
	if store.DialectOf(d.src) != store.DialectSQLite {
		return nil, api.ErrNotSQLite
	}
	checks, err := store.Preflight(ctx, dsn)
	if err != nil {
		return nil, err
	}
	out := make([]api.DatabaseCheck, 0, len(checks))
	for _, c := range checks {
		out = append(out, api.DatabaseCheck{Name: c.Name, Detail: c.Detail, OK: c.OK})
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

func (d *databaseService) Backup(ctx context.Context) (api.DatabaseBackup, error) {
	w := store.BackupWriter(d.src)
	if w == nil {
		return api.DatabaseBackup{}, api.ErrNotSQLite
	}
	bk, err := w.WriteBackup(ctx, d.backupD())
	if err != nil {
		return api.DatabaseBackup{}, err
	}
	out := api.DatabaseBackup{Path: bk.Path, Bytes: bk.Bytes, WrittenAt: bk.WrittenAt}

	d.mu.Lock()
	d.backup = &out
	d.mu.Unlock()
	return out, nil
}

func (d *databaseService) Migrate(_ context.Context, dsn string) error {
	// The complete gate runs before the process-level request is queued.
	d.mu.Lock()
	switch {
	case store.DialectOf(d.src) != store.DialectSQLite:
		d.mu.Unlock()
		return api.ErrNotSQLite
	case d.running:
		d.mu.Unlock()
		return api.ErrMigrationRunning
	case config.PinnedByEnv("DATABASE_URL"):
		d.mu.Unlock()
		return api.ErrDatabaseURLPinned
	case d.preflighted != dsn:
		d.mu.Unlock()
		return api.ErrPreflightFailed
	case d.backup == nil:
		d.mu.Unlock()
		return api.ErrNoBackup
	case d.requestMigration == nil:
		d.mu.Unlock()
		return api.ErrMigrationUnavailable
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

// Switchover remains only for compatibility with clients generated from the former
// two-call flow. The atomic path writes the bootstrap file itself after parity. Refuse
// unless this process holds the exact old-style verified state for this target.
func (d *databaseService) Switchover(_ context.Context, dsn string) error {
	if store.DialectOf(d.src) != store.DialectSQLite {
		return api.ErrNotSQLite
	}
	d.mu.Lock()
	verified := !d.running && d.phase == "verified" && d.parity == "match" &&
		d.preflighted == dsn && d.backup != nil
	d.mu.Unlock()
	if !verified {
		return api.ErrMigrationNotVerified
	}
	if config.PinnedByEnv("DATABASE_URL") {
		return api.ErrDatabaseURLPinned
	}
	return config.UpdateBootstrapFile(d.dataDir, map[string]string{"DATABASE_URL": dsn})
}

// emit announces admission-state changes. It does not claim copy progress: once the
// request is accepted the generation drains, and the reconnecting status endpoint is
// the source of truth for either the new PostgreSQL generation or a carried failure.
func (d *databaseService) emit(st api.DatabaseStatus) {
	if d.bus == nil {
		return
	}
	d.bus.Publish(events.Event{Type: "database", Payload: api.DatabaseEvent{
		Phase: st.Phase, Parity: st.Parity, Tables: st.Tables, Error: st.Error,
	}})
}
