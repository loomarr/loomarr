package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

const downloadFailureSelect = `SELECT item_key, provider, source_id, remote_id, url, duration_ms, height, gap,
	failures, first_failed_at, last_failed_at, retry_at, last_error FROM filler_download_failures`

// An item's key separates its parts with NUL, which Postgres text cannot hold, so the row stores
// the key with the unit separator instead (migration 00249). Every read and write goes through
// these two, and callers only ever see DownloadItem.Key().
func storedKey(key string) string { return strings.ReplaceAll(key, "\x00", "\x1f") }

func itemKey(stored string) string { return strings.ReplaceAll(stored, "\x1f", "\x00") }

func scanDownloadFailure(sc scannable) (acquisition.DownloadFailure, error) {
	var f acquisition.DownloadFailure
	var key string
	var first, last, retry int64
	if err := sc.Scan(&key, &f.Kind, &f.SourceID, &f.RemoteID, &f.URL, &f.DurationMS, &f.Height, &f.Gap,
		&f.Failures, &first, &last, &retry, &f.LastError); err != nil {
		return acquisition.DownloadFailure{}, err
	}
	f.FirstFailedAt, f.LastFailedAt, f.RetryAt = fromEpoch(first), fromEpoch(last), fromEpoch(retry)
	return f, nil
}

// RecordDownloadFailure counts one failed attempt of an item and schedules the next (#3855). The
// count, the timestamps and the schedule change together in one transaction, so two failures
// recorded at once are two failures.
func (s *sqlStore) RecordDownloadFailure(ctx context.Context, item acquisition.DownloadItem, cause string, at time.Time) (acquisition.DownloadFailure, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return acquisition.DownloadFailure{}, fmt.Errorf("begin record download failure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	key := storedKey(item.Key())
	failure, err := scanDownloadFailure(tx.QueryRowContext(ctx, s.ph(storedb.LockRows(s.dialect, downloadFailureSelect+` WHERE item_key = ?`)), key))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		failure = acquisition.DownloadFailure{DownloadItem: item, FirstFailedAt: at}
	case err != nil:
		return acquisition.DownloadFailure{}, fmt.Errorf("read download failure: %w", err)
	}
	if failure.FirstFailedAt.IsZero() {
		failure.FirstFailedAt = at
	}
	failure.DownloadItem = item
	failure.Failures++
	failure.LastFailedAt = at
	failure.RetryAt = acquisition.DownloadRetryAt(failure.Failures, at)
	failure.LastError = cause
	if err := s.writeDownloadFailure(ctx, tx, key, failure); err != nil {
		return acquisition.DownloadFailure{}, err
	}
	if err := tx.Commit(); err != nil {
		return acquisition.DownloadFailure{}, fmt.Errorf("commit download failure: %w", err)
	}
	return failure, nil
}

// DeferDownload schedules an item's next attempt for a temporary cause without counting a failure.
func (s *sqlStore) DeferDownload(ctx context.Context, item acquisition.DownloadItem, reason string, retryAt, at time.Time) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin defer download: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	key := storedKey(item.Key())
	failure, err := scanDownloadFailure(tx.QueryRowContext(ctx, s.ph(storedb.LockRows(s.dialect, downloadFailureSelect+` WHERE item_key = ?`)), key))
	if errors.Is(err, sql.ErrNoRows) {
		failure = acquisition.DownloadFailure{}
	} else if err != nil {
		return fmt.Errorf("read download failure: %w", err)
	}
	failure.DownloadItem = item
	// A rest already running is never cut short by a wait for room.
	if failure.Failures < acquisition.DownloadAttempts || retryAt.After(failure.RetryAt) {
		failure.RetryAt = retryAt
	}
	failure.LastError = reason
	if err := s.writeDownloadFailure(ctx, tx, key, failure); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deferred download: %w", err)
	}
	return nil
}

func (s *sqlStore) writeDownloadFailure(ctx context.Context, tx storedb.Tx, key string, f acquisition.DownloadFailure) error {
	_, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_download_failures
		(item_key, provider, source_id, remote_id, url, duration_ms, height, gap,
		 failures, first_failed_at, last_failed_at, retry_at, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_key) DO UPDATE SET
		 provider=excluded.provider, source_id=excluded.source_id, remote_id=excluded.remote_id, url=excluded.url,
		 duration_ms=excluded.duration_ms, height=excluded.height, gap=excluded.gap,
		 failures=excluded.failures, first_failed_at=excluded.first_failed_at,
		 last_failed_at=excluded.last_failed_at, retry_at=excluded.retry_at, last_error=excluded.last_error`),
		key, f.Kind, f.SourceID, f.RemoteID, f.URL, f.DurationMS, f.Height, f.Gap,
		f.Failures, epoch(f.FirstFailedAt), epoch(f.LastFailedAt), epoch(f.RetryAt), f.LastError)
	if err != nil {
		return fmt.Errorf("write download failure: %w", err)
	}
	return nil
}

// ClearDownload forgets an item that downloaded. An unknown key is not an error.
func (s *sqlStore) ClearDownload(ctx context.Context, key string) error {
	if _, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM filler_download_failures WHERE item_key = ?`), storedKey(key)); err != nil {
		return fmt.Errorf("clear download failure: %w", err)
	}
	return nil
}

// RestingDownloads is every remembered item whose next attempt is after at.
func (s *sqlStore) RestingDownloads(ctx context.Context, at time.Time) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT item_key, retry_at FROM filler_download_failures WHERE retry_at > ?`), epoch(at))
	if err != nil {
		return nil, fmt.Errorf("list resting downloads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]time.Time{}
	for rows.Next() {
		var key string
		var retry int64
		if err := rows.Scan(&key, &retry); err != nil {
			return nil, fmt.Errorf("scan resting download: %w", err)
		}
		out[itemKey(key)] = fromEpoch(retry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list resting downloads: %w", err)
	}
	return out, nil
}

// ClaimDueDownloads returns up to limit items due at at, oldest first, and moves each one's next
// attempt to until in the same transaction.
func (s *sqlStore) ClaimDueDownloads(ctx context.Context, at, until time.Time, limit int) ([]acquisition.DownloadFailure, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim downloads: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, s.ph(storedb.LockRowsSkipLocked(s.dialect, downloadFailureSelect+
		` WHERE retry_at <= ? ORDER BY retry_at, item_key LIMIT ?`)), epoch(at), limit)
	if err != nil {
		return nil, fmt.Errorf("list due downloads: %w", err)
	}
	var due []acquisition.DownloadFailure
	for rows.Next() {
		f, err := scanDownloadFailure(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan due download: %w", err)
		}
		due = append(due, f)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("list due downloads: %w", err)
	}
	_ = rows.Close()
	for _, f := range due {
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_download_failures SET retry_at = ? WHERE item_key = ?`),
			epoch(until), storedKey(f.Key())); err != nil {
			return nil, fmt.Errorf("claim download: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim downloads: %w", err)
	}
	return due, nil
}
