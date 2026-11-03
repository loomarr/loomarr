package storedb

import "context"

// Row locks and transaction-scoped locks exist on Postgres only. SQLite admits one writer at a time,
// so a read-decide-write inside one transaction already excludes every other writer and the lock
// clause would be a syntax error there. These helpers are the one place that difference lives
// (docs/design/decisions/0001-postgres-and-sqlite.md): a call site names the lock it wants and the
// dialect decides whether it is spelled.

// ByDialect returns the SQL spelled for d, for the rare statement whose syntax genuinely differs
// between backends (a JSON accessor, a cast). Both spellings stay visible at the call site; the
// caller states in a comment why they differ. Any dialect other than Postgres takes the SQLite text.
func ByDialect(d Dialect, sqlite, postgres string) string {
	if d == DialectPostgres {
		return postgres
	}
	return sqlite
}

// LockRows appends a row lock to a SELECT: ` FOR UPDATE` on Postgres, the query unchanged on SQLite.
func LockRows(d Dialect, query string) string {
	if d != DialectPostgres {
		return query
	}
	return query + ` FOR UPDATE`
}

// LockRowsShared appends ` FOR SHARE` on Postgres: the caller waits on a concurrent writer's row lock
// and re-reads the row without taking the exclusive lock itself. The query is unchanged on SQLite.
func LockRowsShared(d Dialect, query string) string {
	if d != DialectPostgres {
		return query
	}
	return query + ` FOR SHARE`
}

// LockRowsOf is LockRows restricted to the listed tables or aliases of a join
// (`FOR UPDATE OF i, g`), so the joined rows the caller only reads stay unlocked.
func LockRowsOf(d Dialect, query, aliases string) string {
	if d != DialectPostgres {
		return query
	}
	return query + ` FOR UPDATE OF ` + aliases
}

// LockRowsSkipLocked appends ` FOR UPDATE SKIP LOCKED` on Postgres, so concurrent claimers each take
// distinct rows instead of waiting on one another; the query is unchanged on SQLite.
func LockRowsSkipLocked(d Dialect, query string) string {
	if d != DialectPostgres {
		return query
	}
	return query + ` FOR UPDATE SKIP LOCKED`
}

// XactLock runs lockSQL, a transaction-scoped Postgres advisory lock statement such as
// `SELECT pg_advisory_xact_lock(...)`, on q (which must be the transaction the lock is for). On
// SQLite it does nothing. WithPostgresAdvisoryLock is the session-scoped counterpart for a
// critical section that spans several pooled calls; XactLock is for a section bounded by one
// transaction, which releases the lock at commit or rollback.
func XactLock(ctx context.Context, d Dialect, q Querier, lockSQL string, args ...any) error {
	if d != DialectPostgres {
		return nil
	}
	_, err := q.ExecContext(ctx, lockSQL, args...)
	return err
}
