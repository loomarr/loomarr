package store

import (
	"context"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// Observability counts (§17 /metrics state gauges). These are read on scrape by
// the metrics collector, never on the write path — a handful of cheap grouped
// COUNTs keeps the gauges honest without instrumenting every mutation. The SQL
// is dialect-neutral (plain GROUP BY / COUNT), so one implementation on sqlStore
// serves both backends and the one conformance suite covers both.

// CountTitlesByState returns the number of provisioning records in each state.
// States with no rows are absent from the map; the collector zero-fills the
// known set so a state that empties out reports 0 rather than vanishing.
func (s *sqlStore) CountTitlesByState(ctx context.Context) (map[provision.State]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM titles GROUP BY state`)
	if err != nil {
		return nil, fmt.Errorf("count titles by state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[provision.State]int)
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return nil, fmt.Errorf("scan title-state count: %w", err)
		}
		out[provision.State(state)] = n
	}
	return out, rows.Err()
}

// CountJobsByStatus returns the number of suggester jobs in each status
// (queued/running/done/failed) — the queue-depth gauge (§17).
func (s *sqlStore) CountJobsByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count jobs by status: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int)
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("scan job-status count: %w", err)
		}
		out[status] = n
	}
	return out, rows.Err()
}

// OldestProposalJobsByStatus returns the oldest retained caller-owned Proposal
// Job creation time for each nonterminal status. Re-curation Jobs have a
// different operator lifecycle and do not enter the First-channel health view.
func (s *sqlStore) OldestProposalJobsByStatus(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, MIN(created_at) FROM jobs
		WHERE kind = 'suggest' AND status IN ('queued', 'running') GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("oldest Proposal Jobs by status: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]time.Time)
	for rows.Next() {
		var status string
		var createdAt int64
		if err := rows.Scan(&status, &createdAt); err != nil {
			return nil, fmt.Errorf("scan oldest Proposal Job: %w", err)
		}
		out[status] = fromEpoch(createdAt)
	}
	return out, rows.Err()
}

// CountProposalJobAttemptsByStatus counts durable terminal Attempt history for
// caller-owned Proposal Jobs. Running Attempts are represented by the current
// Job status/age gauges instead of a second, potentially contradictory gauge.
func (s *sqlStore) CountProposalJobAttemptsByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.status, COUNT(*)
		FROM proposal_job_attempts a JOIN jobs j ON j.id = a.job_id
		WHERE j.kind = 'suggest' AND a.status IN ('succeeded', 'failed', 'interrupted')
		GROUP BY a.status`)
	if err != nil {
		return nil, fmt.Errorf("count Proposal Job Attempts by status: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int)
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("scan Proposal Job Attempt count: %w", err)
		}
		out[status] = n
	}
	return out, rows.Err()
}

// CountFailedProposalJobsByCode counts retained failed caller-owned Proposal
// Jobs. Returning raw values here keeps policy out of persistence; the metrics
// module owns the closed public label set and maps every unknown value to other.
func (s *sqlStore) CountFailedProposalJobsByCode(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT failure_code, COUNT(*) FROM jobs
		WHERE kind = 'suggest' AND status = 'failed' GROUP BY failure_code`)
	if err != nil {
		return nil, fmt.Errorf("count failed Proposal Jobs by code: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int)
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return nil, fmt.Errorf("scan failed Proposal Job count: %w", err)
		}
		out[code] = n
	}
	return out, rows.Err()
}

// CountActiveSessions returns the number of unexpired sessions as of now — the
// active-sessions gauge (§17). Mirrors the freshness predicate GetSession uses
// (expires_at strictly greater than now).
func (s *sqlStore) CountActiveSessions(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.ph(
		`SELECT COUNT(*) FROM sessions WHERE expires_at > ?`), epoch(now)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active sessions: %w", err)
	}
	return n, nil
}
