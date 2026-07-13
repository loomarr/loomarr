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
RETURNING t.key, t.title_json, t.state, t.library_id, t.requested_at, t.deadline, t.attempts, t.last_error, t.updated_at`

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
	return &sqlStore{db: db, ph: pgPlaceholders, claimSQL: postgresClaimSQL}, nil
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
