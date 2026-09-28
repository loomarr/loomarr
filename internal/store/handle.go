package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// Querier runs statements, on the pool or inside one transaction. *sql.DB and *sql.Tx both
// satisfy it, so a helper written against it runs either way.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Tx is one transaction begun through Handle.Begin. Rollback after Commit is a no-op, so callers
// defer it straight after Begin.
type Tx interface {
	Querier
	Commit() error
	Rollback() error
}

// Handle is the core store's database handle, lent to the stores that extend it
// (internal/fillerstore). Those stores keep their own tables but run on this handle, and Begin
// is the one way to start a transaction: a write spanning core and extension tables begins here,
// passes the Tx to both halves, and commits or rolls back as one.
//
// ⚠ For extension stores only, and deliberately NOT on the Store interface, for PoolOf's reason:
// everything else goes through Store's methods, which own the tables' invariants.
type Handle interface {
	// Querier runs a statement on the pool, outside any transaction.
	Querier
	// Begin starts the one kind of transaction both halves use.
	Begin(ctx context.Context) (Tx, error)
	// Dialect names the backend, for the few statements that differ.
	Dialect() Dialect
	// Rebind rewrites a query written with ? markers into the backend's placeholder style.
	Rebind(query string) string
	// Clips writes clip columns and tags inside tx, which Begin started. Extension stores write
	// the clips table only through it.
	Clips(tx Tx) ClipTx
}

// HandleOf lends st's database handle to an extension store, or returns nil for a non-SQL store.
func HandleOf(st Store) Handle {
	s, ok := adapterOf(st)
	if !ok {
		return nil
	}
	return sqlHandle{s}
}

// adapterOf returns the SQL adapter behind st, looking through a store that extends it (the
// filler store's Core). Backups, migration and invalidation need the adapter itself.
func adapterOf(st Store) (*sqlStore, bool) {
	if ext, ok := st.(interface{ Core() Store }); ok {
		st = ext.Core()
	}
	s, ok := st.(*sqlStore)
	return s, ok
}

type sqlHandle struct{ s *sqlStore }

func (h sqlHandle) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return h.s.db.ExecContext(ctx, query, args...)
}

func (h sqlHandle) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return h.s.db.QueryContext(ctx, query, args...)
}

func (h sqlHandle) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return h.s.db.QueryRowContext(ctx, query, args...)
}

func (h sqlHandle) Begin(ctx context.Context) (Tx, error) { return h.s.db.BeginTx(ctx, nil) }

func (h sqlHandle) Dialect() Dialect { return h.s.dialect }

func (h sqlHandle) Rebind(query string) string { return h.s.ph(query) }

func (h sqlHandle) Clips(tx Tx) ClipTx { return h.s.clipsIn(tx) }

// Scannable is the one method *sql.Row and *sql.Rows share, so one scan function reads either.
type Scannable = scannable

// Epoch and FromEpoch are the store's Unix-seconds time-column codec; EpochNano and FromEpochNano
// are the nanosecond one the decision and safety ledgers use. Exported so extension stores encode
// time exactly as the core does.
func Epoch(t time.Time) int64 { return epoch(t) }

// FromEpoch decodes Epoch.
func FromEpoch(n int64) time.Time { return fromEpoch(n) }

// EpochNano encodes a time as Unix nanoseconds; the zero time encodes to 0.
func EpochNano(t time.Time) int64 { return fillerDecisionEpoch(t) }

// FromEpochNano decodes EpochNano.
func FromEpochNano(n int64) time.Time { return fromFillerDecisionEpoch(n) }

// ClipPipelineSelect and ScanClipPipeline read filler_clip_pipeline rows for the acquisition
// history, which attributes each run's clips by their pipeline rows. The pipeline stays here while
// clip writes and pipeline writes share core transactions; both go when it moves (#1747).
const ClipPipelineSelect = clipPipelineSelect

// ScanClipPipeline reads one row selected by ClipPipelineSelect.
func ScanClipPipeline(sc Scannable) (filler.ClipPipeline, error) { return scanClipPipeline(sc) }
