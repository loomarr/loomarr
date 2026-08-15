package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// Dialect names a backend. It exists so capability checks read as what they mean:
// backend identity used to be inferred by comparing `claimSQL` against the SQLite
// constant, which works but states "this store's claim statement is the SQLite one"
// when it means "this is SQLite" — and would break silently if the two backends ever
// shared a statement.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// sqlStore is the shared implementation over database/sql (§5, §14). Both the
// SQLite and Postgres backends use it; the only dialect-specific piece is the
// ClaimDueTitles SQL, injected as claimSQL. Placeholders differ too (? vs $1),
// so each query is rebound via the backend's placeholder style.
type sqlStore struct {
	db      *sql.DB
	dialect Dialect
	// mu guards onClose only — the pool itself is already goroutine-safe.
	mu      sync.Mutex
	onClose []func() // pre-close hooks, run in order before the pool closes (see OnClose)
	// settingLock is the SQLite implementation of WithSettingLock. SQLite is
	// explicitly single-instance, so the process is the complete lock domain.
	// Postgres does not use this mutex; it takes a database-wide advisory lock.
	settingLock sync.Mutex
	// path is the SQLite file this store is backed by; empty for Postgres. Kept so a
	// caller can find the data directory without also holding the DSN.
	path                 string
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

const settingLockNamespace = "loomarr:setting-workflow:"

// WithSettingLock hides the backend-specific serialization mechanism behind the
// settings store seam. Postgres advisory locks are session-scoped, so the
// callback runs while one dedicated *sql.Conn remains checked out. Ordinary
// store calls inside fn may use any pooled connection: every cooperating replica
// is excluded by the advisory lock key, not by a SQL transaction.
func (s *sqlStore) WithSettingLock(ctx context.Context, key string, fn func(context.Context) error) (retErr error) {
	if key == "" {
		return fmt.Errorf("setting lock key is empty")
	}
	if fn == nil {
		return fmt.Errorf("setting lock callback is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if s.dialect == DialectSQLite {
		s.settingLock.Lock()
		defer s.settingLock.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn(ctx)
	}
	if s.dialect != DialectPostgres {
		return fmt.Errorf("setting lock: unsupported store dialect %q", s.dialect)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("setting lock %q: reserve postgres connection: %w", key, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("setting lock %q: close postgres connection: %w", key, err))
		}
	}()

	lockName := settingLockNamespace + key
	const lockSQL = `SELECT pg_advisory_lock(hashtextextended(current_database() || ':' || $1, 0))`
	if _, err := conn.ExecContext(ctx, lockSQL, lockName); err != nil {
		// Cancellation can race with the server granting the lock. Discarding the
		// session is the only safe answer when acquisition was not acknowledged.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("setting lock %q: acquire postgres advisory lock: %w", key, err)
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
		retErr = errors.Join(retErr, fmt.Errorf("setting lock %q: release postgres advisory lock: %w", key, unlockErr))
	}()

	return fn(ctx)
}

// SettingRow is a persisted override plus its audit metadata (config-design §3).
// UpdatedBy is empty for env/migration/system writes (stored as NULL).
type SettingRow struct {
	Key       string
	Value     string
	UpdatedAt time.Time
	UpdatedBy string // "" ⇒ NULL (no human author)
	// EnvOverride marks a key an admin has taken back from the environment
	// (config-design §3.1): while true, this stored value wins over the env var.
	EnvOverride bool
}

func (s *sqlStore) ListSettings(ctx context.Context) ([]SettingRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, updated_at, updated_by, env_override FROM settings`)
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
		if err := rows.Scan(&r.Key, &r.Value, &epoch, &by, &r.EnvOverride); err != nil {
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
	// ⚠ env_override is deliberately ABSENT from the DO UPDATE list. It is a property of
	// who controls the key, not of the value, and an ordinary save must not disturb it —
	// listing it here would silently re-lock an unlocked key on the operator's very next
	// edit, which is the one moment they are certain to be editing it. SetSettingEnvOverride
	// is the only writer. On INSERT it takes the column default (false), so a first-time
	// value write never claims the key either.
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO settings (key, value, updated_at, updated_by) VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at, updated_by=excluded.updated_by`),
		row.Key, row.Value, at.Unix(), by)
	return err
}

// SetSettingEnvOverride claims a key for the app, or hands it back (config-design §3.1).
//
// Separate from UpsertSetting because the two answer different questions: that one writes a
// value, this one writes who is in charge of it. The row is created if absent — a key can be
// env-pinned with nothing stored, and unlocking it is exactly the case where that happens.
// Handing a key back leaves the stored value intact, so re-locking is reversible.
func (s *sqlStore) SetSettingEnvOverride(ctx context.Context, key string, on bool, seed string, by string) error {
	var author any
	if by != "" {
		author = by
	}
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO settings (key, value, updated_at, updated_by, env_override) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET env_override=excluded.env_override, updated_at=excluded.updated_at, updated_by=excluded.updated_by`),
		key, seed, time.Now().Unix(), author, on)
	return err
}

func (s *sqlStore) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM settings WHERE key = ?`), key)
	return err
}

// Close runs any registered pre-close hooks, then closes the pool.
//
// ⚠ The hooks run BEFORE the pool closes, and that ordering is the whole point. A subsystem
// holding the pool (the background-job engine, §18.1) issues queries while shutting down; if
// the pool goes first, its shutdown fails silently and its goroutines strand. Measured at 4
// leaked goroutines per generation before this existed — a real cost under the §9.2 restart
// loop, which rebuilds in-process.
func (s *sqlStore) Close() error {
	s.mu.Lock()
	hooks := s.onClose
	s.onClose = nil
	s.mu.Unlock()
	for _, h := range hooks {
		h()
	}
	return s.db.Close()
}

// OnClose registers fn to run just BEFORE st's pool is closed. A no-op for a store that is not
// a SQL store, and for a nil fn.
//
// Deliberately a free function taking Store (like DialectOf and PoolOf) rather than an
// interface method: teardown ordering is a composition-root concern, and putting it on the
// port would invite every consumer to register cleanup on a shared object.
func OnClose(st Store, fn func()) {
	if fn == nil {
		return
	}
	s, ok := st.(*sqlStore)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClose = append(s.onClose, fn)
}

// --- scanning helpers ---

// scannable is satisfied by both *sql.Row and *sql.Rows, so the single-row and list reads
// decode identically — a second decoder is how one path grows a field the other forgets.
//
// ⚠ THE ONE NAME FOR THIS SHAPE. The package had four — `scannable`, `rowScanner`
// (fillerpulls.go), `scanner` (images.go) and an anonymous `interface{ Scan(...any) error }`
// inline in clippipeline.go — all structurally identical, so every one of them was already
// satisfied by every other's arguments. Four names for one idea is a tax on reading, not a
// distinction: a reader meeting the third has to prove to themselves it is not subtly
// different. Consolidated 2026-08-10; put new scan helpers on this one.
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
