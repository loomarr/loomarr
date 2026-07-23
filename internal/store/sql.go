package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// sqlStore is the shared implementation over database/sql (§5, §14). Both the
// SQLite and Postgres backends use it; the only dialect-specific piece is the
// ClaimDueTitles SQL, injected as claimSQL. Placeholders differ too (? vs $1),
// so each query is rebound via the backend's placeholder style.
type sqlStore struct {
	db                   *sql.DB
	ph                   placeholder // rebinds ? -> the dialect's placeholder
	claimSQL             string      // dialect-specific ClaimDueTitles statement (already rebound)
	channelClaimSQL      string      // dialect-specific ClaimDueChannels statement (already rebound)
	jobClaimSQL          string      // dialect-specific ClaimDueJobs statement (already rebound)
	scheduledJobClaimSQL string      // dialect-specific ClaimDueScheduledJobs statement (already rebound)
}

// placeholder rewrites a query written with `?` markers into the dialect's
// style. SQLite uses ?; Postgres uses $1, $2, ….
type placeholder func(query string) string

func passthrough(q string) string { return q }

func (s *sqlStore) GetTitle(ctx context.Context, key provision.Key) (provision.Record, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at,
		        progress, eta_text, download_status
		 FROM titles WHERE key = ?`), string(key))
	return scanTitle(row)
}

func (s *sqlStore) UpsertTitle(ctx context.Context, rec provision.Record) error {
	blob, err := json.Marshal(rec.Title)
	if err != nil {
		return fmt.Errorf("marshal title: %w", err)
	}
	// ON CONFLICT ... DO UPDATE is valid on both dialects (§5). Identity is `key`.
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO titles
		   (key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   title_json=excluded.title_json, state=excluded.state, library_id=excluded.library_id,
		   requested_at=excluded.requested_at, deadline=excluded.deadline, attempts=excluded.attempts,
		   last_error=excluded.last_error, updated_at=excluded.updated_at`),
		string(rec.Key), string(blob), string(rec.State), rec.LibraryID,
		epoch(rec.RequestedAt), epoch(rec.Deadline), rec.Attempts, rec.LastError, epoch(rec.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert title %s: %w", rec.Key, err)
	}
	return nil
}

// UpdateTitleProgress writes ONLY the poll-updated download fields for a title, leaving the
// state-machine columns untouched (§18.1). The arr-queue-poll job owns these; keeping the
// write targeted means a concurrent reconcile/scan Upsert never clobbers the latest progress
// and vice versa. A no-op (no matching key) is not an error — a title may have moved to
// available between the poll and the write.
func (s *sqlStore) UpdateTitleProgress(ctx context.Context, key provision.Key, progress float64, eta, status string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE titles SET progress = ?, eta_text = ?, download_status = ? WHERE key = ?`),
		progress, eta, status, string(key))
	if err != nil {
		return fmt.Errorf("update title progress %s: %w", key, err)
	}
	return nil
}

func (s *sqlStore) ListTitlesByState(ctx context.Context, state provision.State) ([]provision.Record, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at,
		        progress, eta_text, download_status
		 FROM titles WHERE state = ? ORDER BY key`), string(state))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanTitles(rows)
}

// ClaimDueTitles runs the dialect-specific claim statement. It selects in-flight
// records (requested/downloading) with deadline at/before now, up to limit, and
// leases each by advancing its deadline to now+lease so it won't be re-claimed
// until the reconciler acts or the lease lapses (§5 concurrency).
// Placeholder order (all dialects): 1=leaseUntil, 2=now, 3=limit.
func (s *sqlStore) ClaimDueTitles(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]provision.Record, error) {
	leaseUntil := epoch(now.Add(lease))
	rows, err := s.db.QueryContext(ctx, s.claimSQL, leaseUntil, epoch(now), limit)
	if err != nil {
		return nil, fmt.Errorf("claim due titles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanTitles(rows)
}

func (s *sqlStore) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT value FROM settings WHERE key = ?`), key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return v, err
}

// SetSetting is the un-audited system write path (instance id, webhook
// timestamps, the §8.1 model selection). It stamps updated_at but leaves
// updated_by NULL — these writes have no human author. The audited admin path is
// UpsertSetting.
func (s *sqlStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`),
		key, value, time.Now().Unix())
	return err
}

// SettingRow is a persisted override plus its audit metadata (config-design §3).
// UpdatedBy is empty for env/migration/system writes (stored as NULL).
type SettingRow struct {
	Key       string
	Value     string
	UpdatedAt time.Time
	UpdatedBy string // "" ⇒ NULL (no human author)
}

func (s *sqlStore) ListSettings(ctx context.Context) ([]SettingRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, updated_at, updated_by FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SettingRow
	for rows.Next() {
		var (
			r     SettingRow
			epoch int64
			by    sql.NullString
		)
		if err := rows.Scan(&r.Key, &r.Value, &epoch, &by); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		if epoch > 0 {
			r.UpdatedAt = time.Unix(epoch, 0).UTC()
		}
		r.UpdatedBy = by.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertSetting is the audited admin write path (config-design §3, §8). It stamps
// updated_at from the row's time and updated_by (empty ⇒ NULL).
func (s *sqlStore) UpsertSetting(ctx context.Context, row SettingRow) error {
	var by any
	if row.UpdatedBy != "" {
		by = row.UpdatedBy
	}
	at := row.UpdatedAt
	if at.IsZero() {
		at = time.Now()
	}
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO settings (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at, updated_by=excluded.updated_by`),
		row.Key, row.Value, at.Unix(), by)
	return err
}

func (s *sqlStore) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM settings WHERE key = ?`), key)
	return err
}

func (s *sqlStore) Close() error { return s.db.Close() }

// --- scanning helpers ---

type scannable interface {
	Scan(dest ...any) error
}

func scanTitle(sc scannable) (provision.Record, error) {
	var (
		rec                        provision.Record
		blob                       string
		reqAt, deadline, updatedAt int64
	)
	err := sc.Scan(&rec.Key, &blob, &rec.State, &rec.LibraryID,
		&reqAt, &deadline, &rec.Attempts, &rec.LastError, &updatedAt,
		&rec.Progress, &rec.ETAText, &rec.DownloadStatus)
	if err == sql.ErrNoRows {
		return provision.Record{}, ErrNotFound
	}
	if err != nil {
		return provision.Record{}, err
	}
	if err := json.Unmarshal([]byte(blob), &rec.Title); err != nil {
		return provision.Record{}, fmt.Errorf("unmarshal title %s: %w", rec.Key, err)
	}
	rec.RequestedAt = fromEpoch(reqAt)
	rec.Deadline = fromEpoch(deadline)
	rec.UpdatedAt = fromEpoch(updatedAt)
	return rec, nil
}

func scanTitles(rows *sql.Rows) ([]provision.Record, error) {
	var out []provision.Record
	for rows.Next() {
		rec, err := scanTitle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// epoch encodes a time as Unix seconds; the zero time encodes to 0 (§3 note).
func epoch(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// fromEpoch decodes Unix seconds; 0 decodes to the zero time.
func fromEpoch(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}
