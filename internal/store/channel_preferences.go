package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecentChannelsKept bounds each person's Recent list. The guide filter and the surf rail's
// "RECENT" group show a handful; the bound keeps the table from growing with every tune.
const RecentChannelsKept = 20

// FavouriteChannel is one channel a person starred.
type FavouriteChannel struct {
	ChannelID string
	AddedAt   time.Time
}

// RecentChannel is one channel a person tuned, at its latest tune.
type RecentChannel struct {
	ChannelID string
	TunedAt   time.Time
}

// UserChannelLists is one person's guide filters: favourites oldest-starred first, recents newest
// first (at most RecentChannelsKept).
type UserChannelLists struct {
	Favourites []FavouriteChannel
	Recent     []RecentChannel
}

// ChannelPreferenceStore keeps per-person channel lists (#1666). Every method is keyed by user id;
// a paired device resolves to its user before it gets here, so the lists follow the person.
type ChannelPreferenceStore interface {
	// AddFavouriteChannel stars a channel. Idempotent: a repeat keeps the first added time.
	// ErrNotFound when the channel doesn't exist.
	AddFavouriteChannel(ctx context.Context, userID, channelID string, at time.Time) error
	// RemoveFavouriteChannel un-stars a channel; removing one that isn't starred is not an error.
	RemoveFavouriteChannel(ctx context.Context, userID, channelID string) error
	// RecordChannelTune moves a channel to the front of the person's recents (never backwards, so
	// a late report from a slow device can't reorder them) and drops all but the newest
	// RecentChannelsKept. ErrNotFound when the channel doesn't exist.
	RecordChannelTune(ctx context.Context, userID, channelID string, at time.Time) error
	// UserChannelLists reads both lists in one call, for the guide's first paint.
	UserChannelLists(ctx context.Context, userID string) (UserChannelLists, error)
}

func (s *sqlStore) AddFavouriteChannel(ctx context.Context, userID, channelID string, at time.Time) error {
	if userID == "" || channelID == "" || at.IsZero() {
		return fmt.Errorf("add favourite channel: user, channel and time are required")
	}
	return s.writeChannelPreference(ctx, "add favourite channel", channelID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, s.ph(`INSERT INTO user_favourite_channels (user_id, channel_id, added_at)
			VALUES (?, ?, ?) ON CONFLICT (user_id, channel_id) DO NOTHING`), userID, channelID, at.UnixMilli())
		return err
	})
}

func (s *sqlStore) RemoveFavouriteChannel(ctx context.Context, userID, channelID string) error {
	if _, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM user_favourite_channels
		WHERE user_id = ? AND channel_id = ?`), userID, channelID); err != nil {
		return fmt.Errorf("remove favourite channel: %w", err)
	}
	return nil
}

func (s *sqlStore) RecordChannelTune(ctx context.Context, userID, channelID string, at time.Time) error {
	if userID == "" || channelID == "" || at.IsZero() {
		return fmt.Errorf("record channel tune: user, channel and time are required")
	}
	return s.writeChannelPreference(ctx, "record channel tune", channelID, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO user_recent_channels (user_id, channel_id, tuned_at)
			VALUES (?, ?, ?) ON CONFLICT (user_id, channel_id) DO UPDATE SET tuned_at =
			CASE WHEN excluded.tuned_at > user_recent_channels.tuned_at
			THEN excluded.tuned_at ELSE user_recent_channels.tuned_at END`),
			userID, channelID, at.UnixMilli()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.ph(`DELETE FROM user_recent_channels WHERE user_id = ?
			AND channel_id NOT IN (SELECT channel_id FROM user_recent_channels WHERE user_id = ?
			ORDER BY tuned_at DESC, channel_id LIMIT ?)`), userID, userID, RecentChannelsKept)
		return err
	})
}

// writeChannelPreference runs one list write under the same channel lock AppendDiscoveryFeedback
// takes: a no-op UPDATE holds the channel row against DeleteChannel until commit (Postgres) or
// takes SQLite's writer lock, so a write either lands before the delete and cascades with it, or
// sees the channel gone and reports ErrNotFound — never a foreign-key error from the race.
func (s *sqlStore) writeChannelPreference(ctx context.Context, op, channelID string, write func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin: %w", op, err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE channels SET id = id WHERE id = ?`), channelID)
	if err != nil {
		return fmt.Errorf("%s: lock channel: %w", op, err)
	}
	matched, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: lock channel affected rows: %w", op, err)
	}
	if matched == 0 {
		return ErrNotFound
	}
	if err := write(tx); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}

func (s *sqlStore) UserChannelLists(ctx context.Context, userID string) (UserChannelLists, error) {
	var out UserChannelLists
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT channel_id, added_at FROM user_favourite_channels
		WHERE user_id = ? ORDER BY added_at, channel_id`), userID)
	if err != nil {
		return out, fmt.Errorf("list favourite channels: %w", err)
	}
	for rows.Next() {
		var f FavouriteChannel
		var added int64
		if err := rows.Scan(&f.ChannelID, &added); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("list favourite channels: %w", err)
		}
		f.AddedAt = time.UnixMilli(added).UTC()
		out.Favourites = append(out.Favourites, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return out, fmt.Errorf("list favourite channels: %w", err)
	}
	rows, err = s.db.QueryContext(ctx, s.ph(`SELECT channel_id, tuned_at FROM user_recent_channels
		WHERE user_id = ? ORDER BY tuned_at DESC, channel_id LIMIT ?`), userID, RecentChannelsKept)
	if err != nil {
		return out, fmt.Errorf("list recent channels: %w", err)
	}
	for rows.Next() {
		var r RecentChannel
		var tuned int64
		if err := rows.Scan(&r.ChannelID, &tuned); err != nil {
			_ = rows.Close()
			return out, fmt.Errorf("list recent channels: %w", err)
		}
		r.TunedAt = time.UnixMilli(tuned).UTC()
		out.Recent = append(out.Recent, r)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return out, fmt.Errorf("list recent channels: %w", err)
	}
	return out, nil
}
