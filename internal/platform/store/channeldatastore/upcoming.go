package channeldatastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// UpcomingAiring is one programme a channel airs within the stored horizon (#4476, scheduling.md
// "Upcoming airings"). Start and End are wall-clock; End is exclusive.
type UpcomingAiring struct {
	ChannelID     string
	Start, End    time.Time
	LibraryItemID string
	// TitleKey is the lineup title the programme belongs to; a series finds its episodes by it.
	TitleKey provision.Key
	// Forecast is true for a programme past the rolling window on air.
	Forecast bool
}

// ReplaceUpcomingAirings replaces every stored upcoming airing of a channel with airings, in one
// transaction, while the channel is still at revision. It reports whether it wrote: a channel
// that moved past revision, or is gone, keeps its rows, since airings were computed from a state
// that is no longer current. Airings with an empty library item id are skipped.
func (s *Store) ReplaceUpcomingAirings(ctx context.Context, channelID string, revision int64, airings []UpcomingAiring) (bool, error) {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("replace upcoming airings %s: %w", channelID, err)
	}
	defer func() { _ = tx.Rollback() }()

	// The row lock (Postgres) or the immediate write lock (SQLite) holds the revision until commit,
	// so a reconcile that commits a newer revision meanwhile waits for this replace, then writes its own.
	var current int64
	err = tx.QueryRowContext(ctx, s.h.Rebind(storedb.LockRows(s.h.Dialect(),
		`SELECT revision FROM channels WHERE id = ?`)), channelID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("replace upcoming airings %s: read revision: %w", channelID, err)
	}
	if current != revision {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, s.h.Rebind(`DELETE FROM channel_upcoming_airings WHERE channel_id = ?`), channelID); err != nil {
		return false, fmt.Errorf("replace upcoming airings %s: delete: %w", channelID, err)
	}
	for _, a := range airings {
		if a.LibraryItemID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO channel_upcoming_airings
			(channel_id, starts_at_ms, ends_at_ms, library_item_id, title_key, forecast, revision)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
			channelID, a.Start.UnixMilli(), a.End.UnixMilli(), a.LibraryItemID, string(a.TitleKey), a.Forecast, revision); err != nil {
			return false, fmt.Errorf("replace upcoming airings %s: insert %s: %w", channelID, a.LibraryItemID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("replace upcoming airings %s: commit: %w", channelID, err)
	}
	return true, nil
}

// UpcomingAiringsForItem returns the stored airings of a library item that have not ended by now,
// on channels neither paused nor detached, earliest first. A movie matches on its own id; a series
// matches its episodes through the title key its titles row holds, the key a lineup names it by.
func (s *Store) UpcomingAiringsForItem(ctx context.Context, libraryItemID string, now time.Time) ([]UpcomingAiring, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT u.channel_id, u.starts_at_ms, u.ends_at_ms,
			u.library_item_id, u.title_key, u.forecast
		FROM channel_upcoming_airings u JOIN channels c ON c.id = u.channel_id
		WHERE u.ends_at_ms > ? AND c.status NOT IN ('paused', 'detached')
		  AND (u.library_item_id = ?
		       OR u.title_key IN (SELECT t.key FROM titles t WHERE t.library_id = ?))
		ORDER BY u.starts_at_ms, u.channel_id`),
		now.UnixMilli(), libraryItemID, libraryItemID)
	if err != nil {
		return nil, fmt.Errorf("upcoming airings for %s: %w", libraryItemID, err)
	}
	defer func() { _ = rows.Close() }()
	out := []UpcomingAiring{}
	for rows.Next() {
		var a UpcomingAiring
		var startMs, endMs int64
		var key string
		if err := rows.Scan(&a.ChannelID, &startMs, &endMs, &a.LibraryItemID, &key, &a.Forecast); err != nil {
			return nil, fmt.Errorf("upcoming airings for %s: scan: %w", libraryItemID, err)
		}
		a.Start, a.End, a.TitleKey = time.UnixMilli(startMs), time.UnixMilli(endMs), provision.Key(key)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("upcoming airings for %s: %w", libraryItemID, err)
	}
	return out, nil
}
