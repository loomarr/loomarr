package storedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

// WithPostgresAdvisoryLock holds one session-scoped advisory lock while fn uses
// ordinary pooled Store calls. It is the settings transition lock; proposal
// approvals use their own transaction-bound ordering in internal/store/approval.go.
//
// Postgres advisory locks are session-scoped, so fn runs while one dedicated *sql.Conn
// remains checked out. Ordinary store calls inside fn may use any pooled connection: every
// cooperating replica is excluded by the advisory lock key, not by a SQL transaction.
func WithPostgresAdvisoryLock(
	ctx context.Context,
	db *sql.DB,
	dialect Dialect,
	label, namespace, key string,
	fn func(context.Context) error,
) (retErr error) {
	if dialect != DialectPostgres {
		return fmt.Errorf("%s: unsupported store dialect %q", label, dialect)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("%s %q: reserve postgres connection: %w", label, key, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("%s %q: close postgres connection: %w", label, key, err))
		}
	}()

	lockName := namespace + key
	const lockSQL = `SELECT pg_advisory_lock(hashtextextended(current_database() || ':' || $1, 0))`
	if _, err := conn.ExecContext(ctx, lockSQL, lockName); err != nil {
		// Cancellation can race with the server granting the lock. Discarding the
		// session is the only safe answer when acquisition was not acknowledged.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("%s %q: acquire postgres advisory lock: %w", label, key, err)
	}

	// Unlock with an independent bounded context. The operation's context may have
	// been cancelled while fn was running; returning the session to the pool with a
	// held advisory lock would silently strand the lock. If explicit unlock cannot
	// be proved, poison the driver connection so database/sql discards the session.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		const unlockSQL = `SELECT pg_advisory_unlock(hashtextextended(current_database() || ':' || $1, 0))`
		var unlocked bool
		unlockErr := conn.QueryRowContext(cleanupCtx, unlockSQL, lockName).Scan(&unlocked)
		if unlockErr == nil && unlocked {
			return
		}
		if unlockErr == nil {
			unlockErr = errors.New("postgres session did not own advisory lock")
		}
		// Returning driver.ErrBadConn from Raw tells database/sql not to reuse the
		// underlying session; Postgres releases session locks when it disconnects.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		retErr = errors.Join(retErr, fmt.Errorf("%s %q: release postgres advisory lock: %w", label, key, unlockErr))
	}()

	return fn(ctx)
}
