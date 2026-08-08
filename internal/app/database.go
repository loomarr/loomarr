package app

import (
	"context"
	"fmt"
	"strings"
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
// State is in-memory and per-process on purpose. A migration is a single supervised
// session, not a resumable job: if the process restarts mid-migration the operator is
// back on SQLite with a half-written target, and the correct next step is to wipe the
// target and start over — which is exactly what losing the state forces. Persisting it
// would let a stale "preflight passed" from before a restart authorize a migration
// against a target that has since changed.
type databaseService struct {
	src     store.Store
	dataDir string
	backupD func() string // reads backup.dir from settings at call time (hot-applied)
	bus     *events.Bus

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

func (d *databaseService) Migrate(ctx context.Context, dsn string) error {
	// --- the gate, before anything is opened ---
	d.mu.Lock()
	switch {
	case store.DialectOf(d.src) != store.DialectSQLite:
		d.mu.Unlock()
		return api.ErrNotSQLite
	case d.running:
		d.mu.Unlock()
		return api.ErrMigrationRunning
	case d.preflighted != dsn:
		d.mu.Unlock()
		return api.ErrPreflightFailed
	case d.backup == nil:
		d.mu.Unlock()
		return api.ErrNoBackup
	}
	d.running = true
	d.phase = "migrating"
	d.parity = "unknown"
	d.lastErr = ""
	d.tables = nil
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	// autoMigrate builds the destination schema from the same embedded migrations, which
	// is what makes the destination's catalog authoritative for the table list.
	dst, err := store.Open(ctx, dsn, true)
	if err != nil {
		return d.fail(fmt.Errorf("open target: %w", err))
	}
	defer func() { _ = dst.Close() }()

	if _, err := store.MigrateData(ctx, d.src, dst, d.publish); err != nil {
		return d.fail(err)
	}

	// Parity is a SEPARATE re-count of both sides — see store.VerifyParity. A copy that
	// reports success is a claim; this is the evidence.
	stats, err := store.VerifyParity(ctx, d.src, dst)
	if err != nil {
		return d.fail(fmt.Errorf("verify: %w", err))
	}
	if bad := store.ParityMismatches(stats); len(bad) > 0 {
		names := make([]string, 0, len(bad))
		for _, b := range bad {
			names = append(names, fmt.Sprintf("%s (%d→%d)", b.Table, b.Source, b.Copied))
		}
		d.mu.Lock()
		d.parity = "mismatch"
		d.mu.Unlock()
		return d.fail(fmt.Errorf("row counts do not match: %s", strings.Join(names, ", ")))
	}

	d.mu.Lock()
	d.phase = "verified"
	d.parity = "match"
	d.tables = toTableViews(stats)
	st := d.statusLocked()
	d.mu.Unlock()
	d.emit(st)
	return nil
}

// Switchover records the new DSN for the next boot. It does NOT restart: restarting is
// the operator's call (and the existing restart control's job), and doing it implicitly
// would take the instance down at a moment they did not choose.
func (d *databaseService) Switchover(_ context.Context, dsn string) error {
	if config.PinnedByEnv("DATABASE_URL") {
		// An env pin always wins at boot, so writing the file would produce a switch
		// that silently does not happen — the exact failure the bootstrap-file tier
		// exists to avoid claiming.
		return fmt.Errorf("DATABASE_URL is pinned by the environment — change it there instead; " +
			"a value written here would be ignored at boot")
	}
	return config.WriteBootstrapFile(d.dataDir, map[string]string{"DATABASE_URL": dsn})
}

func (d *databaseService) fail(err error) error {
	d.mu.Lock()
	d.phase = "failed"
	d.lastErr = err.Error()
	st := d.statusLocked()
	d.mu.Unlock()
	d.emit(st)
	return err
}

// publish mirrors copy progress into the service's state and onto the bus.
func (d *databaseService) publish(p store.MigrationProgress) {
	d.mu.Lock()
	d.tables = toTableViews(p.Tables)
	st := d.statusLocked()
	d.mu.Unlock()
	d.emit(st)
}

// emit publishes a `database` frame. Like every other frame this is a latency
// optimization — GET /v1/system/database is the truth on reconnect (§8) — so a dropped
// frame costs a stale progress bar, never a wrong outcome.
func (d *databaseService) emit(st api.DatabaseStatus) {
	if d.bus == nil {
		return
	}
	d.bus.Publish(events.Event{Type: "database", Payload: api.DatabaseEvent{
		Phase:  st.Phase,
		Parity: st.Parity,
		Tables: st.Tables,
		Error:  st.Error,
	}})
}

func toTableViews(stats []store.TableStat) []api.DatabaseTable {
	out := make([]api.DatabaseTable, 0, len(stats))
	for _, s := range stats {
		out = append(out, api.DatabaseTable{Table: s.Table, Source: s.Source, Copied: s.Copied})
	}
	return out
}
