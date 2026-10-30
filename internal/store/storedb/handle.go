package storedb

import (
	"context"
	"database/sql"
)

// AdvisoryLocker is the cross-replica lock a store package needs and nothing else. Only the
// Postgres backend can honour it (SQLite is single-instance, so a process-local lock is the whole
// domain; see KeyedLock), and an implementation on SQLite returns an error rather than pretending.
type AdvisoryLocker interface {
	// Dialect names the backend, so a caller can choose its SQLite fallback.
	Dialect() Dialect
	// WithAdvisoryLock runs fn while holding one advisory lock; see WithPostgresAdvisoryLock.
	WithAdvisoryLock(ctx context.Context, label, namespace, key string, fn func(context.Context) error) error
}

// Handle is everything a store package needs from the persistence kernel to run its own
// statements and its own transactions: the pool, Begin, the dialect, placeholder rebinding and the
// advisory lock. It names no table and no Loomarr type, so a cluster package (identity, catalog,
// ...) can take it without importing internal/store.
//
// A transaction begun here is an ordinary Tx. A cluster store that must join a caller's
// transaction takes the Querier it was given instead of calling Begin; Rebind and Dialect are
// stateless, so they work the same on either.
type Handle interface {
	// Querier runs a statement on the pool, outside any transaction.
	Querier
	AdvisoryLocker
	// Begin starts a transaction on the pool.
	Begin(ctx context.Context) (Tx, error)
	// Rebind rewrites a query written with ? markers into the backend's placeholder style.
	Rebind(query string) string
}

// DB is the concrete Handle over one *sql.DB. internal/store builds one from its pool; a cluster
// store's tests can build one from any pool with NewDB.
type DB struct {
	db      *sql.DB
	dialect Dialect
	rebind  Placeholder
}

var _ Handle = DB{}

// NewDB binds a pool to its backend's dialect and placeholder style.
func NewDB(db *sql.DB, dialect Dialect, rebind Placeholder) DB {
	return DB{db: db, dialect: dialect, rebind: rebind}
}

// ExecContext runs a statement on the pool.
func (d DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

// QueryContext runs a query on the pool.
func (d DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

// QueryRowContext runs a single-row query on the pool.
func (d DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

// Begin starts a transaction with the driver's default options.
func (d DB) Begin(ctx context.Context) (Tx, error) { return d.db.BeginTx(ctx, nil) }

// Dialect names the backend.
func (d DB) Dialect() Dialect { return d.dialect }

// Rebind rewrites ? markers into the backend's placeholder style.
func (d DB) Rebind(query string) string { return d.rebind(query) }

// WithAdvisoryLock is WithPostgresAdvisoryLock on this pool and dialect.
func (d DB) WithAdvisoryLock(ctx context.Context, label, namespace, key string, fn func(context.Context) error) error {
	return WithPostgresAdvisoryLock(ctx, d.db, d.dialect, label, namespace, key, fn)
}
