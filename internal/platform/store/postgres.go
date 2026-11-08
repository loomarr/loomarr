package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql shim (§14)
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Postgres claim: FOR UPDATE SKIP LOCKED so concurrent replicas never both
// claim the same due row (§5). The CTE locks the due rows; the UPDATE leases
// them (deadline := now+lease) so a locked-and-claimed row won't re-match once
// the lock is released. RETURNs the leased deadline (see sqliteClaimSQL note).
// Placeholders: $1=leaseUntil, $2=now, $3=limit.
const postgresClaimSQL = `
WITH due AS (
    SELECT key FROM titles
    WHERE state IN ('wanted','requested','downloading') AND deadline <= $2 AND deadline > 0
    ORDER BY deadline
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE titles t SET deadline = $1
FROM due WHERE t.key = due.key
RETURNING t.key, t.title_json, t.state, t.library_id, t.requested_at, t.deadline, t.attempts, t.last_error, t.updated_at,
          t.progress, t.eta_text, t.download_status, t.arrived_at, t.episodes_have, t.episodes_wanted` // titleColumns, aliased

// Postgres channel claim: FOR UPDATE SKIP LOCKED so two replicas never reconcile
// the same channel (§18 single-leader-per-channel). Keyed on reconcile_deadline,
// excludes detached + paused channels (both off the sweep). Placeholders:
// $1=leaseUntil, $2=now, $3=limit.
//
// ⚠ No `reconcile_deadline > 0` guard, for the reason spelled out on the SQLite statement:
// a zero deadline means DUE NOW (§9 V54). One suite, two backends — the predicates must match.
const postgresChannelClaimSQL = `
WITH due AS (
    SELECT id FROM channels
    WHERE status NOT IN ('detached', 'paused') AND reconcile_deadline <= $2
    ORDER BY reconcile_deadline
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE channels c SET reconcile_deadline = $1, revision = c.revision + 1
FROM due WHERE c.id = due.id
RETURNING c.id, c.intent_ref, c.intent_json, c.requested_by, c.name, c.number, c.grp, c.logo, c.strategy,
          c.tunarr_id, c.status, c.shuffle_seed, c.lineup_json, c.desired_json, c.policy_json,
          c.broadcast_codec, c.playout_anchor, c.reconcile_deadline, c.updated_at, c.revision, c.created_at` // channelColumns, aliased

// Postgres pool sizing (#2446). database/sql's defaults are no ceiling and only 2 idle connections,
// so a burst opened Postgres connections (a server process each) and closed all but two afterwards,
// paying the connect again on the next burst. One replica's peak is River (5 workers plus its
// producer, elector and completer) and the HTTP requests in flight; 20 covers that with room, and
// three replicas stay well inside Postgres's default max_connections of 100. The invalidation
// listener dials its own connection outside this pool.
const (
	postgresMaxOpenConns    = 20
	postgresConnMaxIdleTime = 5 * time.Minute // a quiet install gives its connections back
	postgresConnMaxLifetime = time.Hour       // recycle across a server restart or failover
)

func configurePostgresPool(db *sql.DB) {
	db.SetMaxOpenConns(postgresMaxOpenConns)
	db.SetMaxIdleConns(postgresMaxOpenConns)
	db.SetConnMaxIdleTime(postgresConnMaxIdleTime)
	db.SetConnMaxLifetime(postgresConnMaxLifetime)
}

// openPostgres opens a Postgres connection via pgx's stdlib shim and returns a
// store wired with the SKIP-LOCKED claim SQL and $N placeholder rebinding.
// Conformance (incl. concurrent claim) is proven in Phase 4 via testcontainers.
func openPostgres(ctx context.Context, dsn string) (*sqlStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	configurePostgresPool(db)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return bindClusters(&sqlStore{db: db, dialect: DialectPostgres, dsn: dsn, ph: storedb.PgPlaceholders, claimSQL: postgresClaimSQL, channelClaimSQL: postgresChannelClaimSQL}), nil
}
