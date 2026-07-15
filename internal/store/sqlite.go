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

// SQLite channel claim: same guarded-UPDATE lease as titles, keyed on
// reconcile_deadline and excluding detached channels (§9/§18). RETURNs the full
// channel column set (channelSelect order) so scanChannel serves it.
// Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
const sqliteChannelClaimSQL = `
UPDATE channels SET reconcile_deadline = ?1
WHERE id IN (
    SELECT id FROM channels
    WHERE status <> 'detached' AND reconcile_deadline <= ?2 AND reconcile_deadline > 0
    ORDER BY reconcile_deadline LIMIT ?3
)
RETURNING id, intent_ref, name, number, grp, logo, strategy, filler_ref, tunarr_id,
          status, shuffle_seed, lineup_json, desired_json, policy_json, reconcile_deadline, updated_at`

// SQLite job claim: lease due queued jobs (§8). Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
const sqliteJobClaimSQL = `
UPDATE jobs SET deadline = ?1
WHERE id IN (
    SELECT id FROM jobs
    WHERE status = 'queued' AND deadline <= ?2 AND deadline > 0
    ORDER BY deadline LIMIT ?3
)
RETURNING id, kind, status, intent_json, intent_hash, created_by, last_error,
          deadline, attempts, created_at, updated_at`

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
	return &sqlStore{db: db, ph: passthrough, claimSQL: sqliteClaimSQL, channelClaimSQL: sqliteChannelClaimSQL, jobClaimSQL: sqliteJobClaimSQL}, nil
}
