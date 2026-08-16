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
RETURNING key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at,
          progress, eta_text, download_status`

// SQLite channel claim: same guarded-UPDATE lease as titles, keyed on
// reconcile_deadline and excluding detached + paused channels (§9/§18) — both are
// off the sweep (detached = unmanaged; paused = deliberately off-air). RETURNs the
// full channel column set (channelSelect order) so scanChannel serves it.
// Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
//
// ⚠ **There is deliberately NO `reconcile_deadline > 0` guard. A zero deadline means DUE NOW**
// (§9 V54), and re-adding that guard reintroduces a permanent strand. The deadline's only writer
// is the LAST step of a SUCCESSFUL reconcile, so a channel whose first reconcile failed keeps 0 —
// with the guard it was invisible to the sweep forever, stuck in `building`, never pushed to
// Tunarr, while the binder's comment promised "the sweep retries". The sweep retried everything
// except the one case it existed for. Channels that opt out of reconciliation say so in `status`;
// that is the only exclusion, and a magic deadline value must never become a second one.
const sqliteChannelClaimSQL = `
UPDATE channels SET reconcile_deadline = ?1, revision = revision + 1
WHERE id IN (
    SELECT id FROM channels
    WHERE status NOT IN ('detached', 'paused') AND reconcile_deadline <= ?2
    ORDER BY reconcile_deadline LIMIT ?3
)
RETURNING id, intent_ref, name, number, grp, logo, strategy, filler_ref, tunarr_id,
          status, shuffle_seed, lineup_json, desired_json, policy_json, broadcast_codec,
          reconcile_deadline, updated_at, revision`

// SQLite job claim: lease due queued jobs (§8). Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
const sqliteJobClaimSQL = `
UPDATE jobs SET deadline = ?1, status = 'running', attempts = attempts + 1, updated_at = ?2
WHERE id IN (
    SELECT id FROM jobs
    WHERE status IN ('queued', 'running') AND deadline <= ?2 AND deadline > 0
    ORDER BY deadline LIMIT ?3
)
RETURNING id, kind, status, intent_json, intent_hash, created_by, last_error, failure_code,
          deadline, attempts, created_at, updated_at`

// sqliteScheduledJobClaimSQL leases every due scheduled job (§18.1): advance next_run to the
// lease so a concurrent tick/replica won't re-claim it until this run finishes and reschedules
// (the scheduler upserts the real next_run after running). Placeholders: 1=leaseUntil, 2=now.
const sqliteScheduledJobClaimSQL = `
UPDATE scheduled_jobs SET next_run = ?1
WHERE name IN (
    SELECT name FROM scheduled_jobs WHERE next_run <= ?2 AND paused = 0
)
RETURNING name, last_run, last_result, last_error, next_run, updated_at, paused`

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
	return &sqlStore{db: db, dialect: DialectSQLite, path: path, ph: passthrough, claimSQL: sqliteClaimSQL, channelClaimSQL: sqliteChannelClaimSQL, jobClaimSQL: sqliteJobClaimSQL, scheduledJobClaimSQL: sqliteScheduledJobClaimSQL}, nil
}

// openSQLiteReadOnly is the migration source opener. Both URI mode=ro and
// PRAGMA query_only are intentional: sql.TxOptions{ReadOnly:true} is advisory in the
// modernc driver, while the migration invariant requires the source to be incapable of
// accepting a write after the live generation has closed.
func openSQLiteReadOnly(ctx context.Context, path string) (*sqlStore, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping read-only sqlite: %w", err)
	}
	return &sqlStore{db: db, dialect: DialectSQLite, path: path, ph: passthrough}, nil
}
