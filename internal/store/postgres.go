package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql shim (§14)
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
          t.progress, t.eta_text, t.download_status`

// Postgres channel claim: FOR UPDATE SKIP LOCKED so two replicas never reconcile
// the same channel (§18 single-leader-per-channel). Keyed on reconcile_deadline,
// excludes detached + paused channels (both off the sweep). Placeholders:
// $1=leaseUntil, $2=now, $3=limit.
const postgresChannelClaimSQL = `
WITH due AS (
    SELECT id FROM channels
    WHERE status NOT IN ('detached', 'paused') AND reconcile_deadline <= $2 AND reconcile_deadline > 0
    ORDER BY reconcile_deadline
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE channels c SET reconcile_deadline = $1
FROM due WHERE c.id = due.id
RETURNING c.id, c.intent_ref, c.name, c.number, c.grp, c.logo, c.strategy, c.filler_ref,
          c.tunarr_id, c.status, c.shuffle_seed, c.lineup_json, c.desired_json, c.policy_json,
          c.reconcile_deadline, c.updated_at`

// Postgres job claim: FOR UPDATE SKIP LOCKED so replicas never run one job twice
// (§8/§18). Placeholders: $1=leaseUntil, $2=now, $3=limit.
const postgresJobClaimSQL = `
WITH due AS (
    SELECT id FROM jobs
    WHERE status = 'queued' AND deadline <= $2 AND deadline > 0
    ORDER BY deadline
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE jobs j SET deadline = $1
FROM due WHERE j.id = due.id
RETURNING j.id, j.kind, j.status, j.intent_json, j.intent_hash, j.created_by, j.last_error,
          j.deadline, j.attempts, j.created_at, j.updated_at`

// postgresScheduledJobClaimSQL leases every due scheduled job (§18.1) via SKIP LOCKED so two
// replicas never both run one job in a tick. Placeholders: $1=leaseUntil, $2=now.
const postgresScheduledJobClaimSQL = `
WITH due AS (
    SELECT name FROM scheduled_jobs
    WHERE next_run <= $2 AND paused = FALSE
    FOR UPDATE SKIP LOCKED
)
UPDATE scheduled_jobs sj SET next_run = $1
FROM due WHERE sj.name = due.name
RETURNING sj.name, sj.last_run, sj.last_result, sj.last_error, sj.next_run, sj.updated_at, sj.paused`

// openPostgres opens a Postgres connection via pgx's stdlib shim and returns a
// store wired with the SKIP-LOCKED claim SQL and $N placeholder rebinding.
// Conformance (incl. concurrent claim) is proven in Phase 4 via testcontainers.
func openPostgres(ctx context.Context, dsn string) (*sqlStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &sqlStore{db: db, dialect: DialectPostgres, ph: pgPlaceholders, claimSQL: postgresClaimSQL, channelClaimSQL: postgresChannelClaimSQL, jobClaimSQL: postgresJobClaimSQL, scheduledJobClaimSQL: postgresScheduledJobClaimSQL}, nil
}

// pgPlaceholders rewrites `?` markers into Postgres `$1, $2, …` in order.
func pgPlaceholders(query string) string {
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
