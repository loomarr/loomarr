package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go driver, no cgo (§5, §16)
)

// SQLite claim: a guarded UPDATE that leases the due rows by pushing their
// deadline to ?1 (now+lease), so they aren't re-claimed until the reconciler
// acts or the lease lapses. A single instance is assumed (§5), so the UPDATE's
// atomicity suffices — no row-locking. RETURNING yields the leased deadline;
// the reconciler acts on the row's state (give-up/retry) and recomputes any new
// deadline itself, so it doesn't depend on the pre-lease value.
// Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
const sqliteClaimSQL = `
UPDATE titles SET deadline = ?1
WHERE key IN (
    SELECT key FROM titles
    WHERE state IN ('wanted','requested','downloading') AND deadline <= ?2 AND deadline > 0
    ORDER BY deadline LIMIT ?3
)
RETURNING key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at`

// openSQLite opens the DB file with WAL + busy_timeout (§5) and returns a store
// wired with the SQLite claim SQL. dsn is the path after the sqlite:// scheme.
func openSQLite(ctx context.Context, path string) (*sqlStore, error) {
	// WAL for concurrent readers; busy_timeout so brief write contention retries
	// instead of erroring immediately.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc's sqlite serializes writes; a single connection avoids WAL write
	// contention surprises in-process. Readers still proceed under WAL.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &sqlStore{db: db, ph: passthrough, claimSQL: sqliteClaimSQL}, nil
}
