package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Viewing is one row of the watch history (#3010): a person watching one programme on one channel
// on one device, from the first playlist poll to the last.
type Viewing struct {
	ID     string
	UserID string
	// Device is what the household calls the device ("Living room TV", "Firefox on macOS").
	Device    string
	ChannelID string
	// ChannelName is the channel's name when the viewing began, so the row reads the same after
	// the channel is renamed or deleted.
	ChannelName string
	// Programme is the title watched, or "" when the guide had none.
	Programme string
	StartedAt time.Time
	EndedAt   time.Time
}

// ViewingQuery pages the watch history newest first. Zero fields do not filter.
type ViewingQuery struct {
	UserID string
	// From and To keep viewings that overlap [From, To).
	From, To time.Time
	// Limit is 1–501.
	Limit int
	// AfterStartedAt and AfterID continue from the last row of the previous page.
	AfterStartedAt time.Time
	AfterID        string
}

// ViewingStore keeps the watch history. It has no retention: rows stay until an admin clears them.
type ViewingStore interface {
	InsertViewing(ctx context.Context, v Viewing) error
	// ExtendViewing moves a viewing's end to endedAt (never earlier than it is) and, when programme
	// is not "", records the programme. It reports false when the row is gone (an admin cleared it).
	ExtendViewing(ctx context.Context, id, programme string, endedAt time.Time) (bool, error)
	ListViewings(ctx context.Context, q ViewingQuery) ([]Viewing, error)
	// DeleteViewings clears one person's history, or everyone's when userID is "", and reports how
	// many rows it removed.
	DeleteViewings(ctx context.Context, userID string) (int, error)
}

func (s *sqlStore) InsertViewing(ctx context.Context, v Viewing) error {
	if v.ID == "" || v.UserID == "" || v.ChannelID == "" || v.StartedAt.Unix() <= 0 {
		return fmt.Errorf("insert viewing: id, person, channel and start are required")
	}
	ended := v.EndedAt
	if ended.Before(v.StartedAt) {
		ended = v.StartedAt
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO viewings (id, user_id, device, channel_id,
		channel_name, programme, started_at, ended_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		v.ID, v.UserID, v.Device, v.ChannelID, v.ChannelName, v.Programme, v.StartedAt.Unix(),
		ended.Unix()); err != nil {
		return fmt.Errorf("insert viewing: %w", err)
	}
	return nil
}

func (s *sqlStore) ExtendViewing(ctx context.Context, id, programme string, endedAt time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE viewings
		SET ended_at = CASE WHEN ended_at < ? THEN ? ELSE ended_at END,
		    programme = CASE WHEN ? = '' THEN programme ELSE ? END
		WHERE id = ?`), endedAt.Unix(), endedAt.Unix(), programme, programme, id)
	if err != nil {
		return false, fmt.Errorf("extend viewing %s: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("extend viewing %s: %w", id, err)
	}
	return n > 0, nil
}

func (s *sqlStore) ListViewings(ctx context.Context, q ViewingQuery) ([]Viewing, error) {
	if q.Limit < 1 || q.Limit > 501 {
		return nil, fmt.Errorf("list viewings: limit %d out of range", q.Limit)
	}
	var where []string
	var args []any
	if q.UserID != "" {
		where = append(where, `user_id = ?`)
		args = append(args, q.UserID)
	}
	if !q.From.IsZero() {
		where = append(where, `ended_at >= ?`)
		args = append(args, q.From.Unix())
	}
	if !q.To.IsZero() {
		where = append(where, `started_at < ?`)
		args = append(args, q.To.Unix())
	}
	if q.AfterID != "" {
		at := q.AfterStartedAt.Unix()
		where = append(where, `(started_at < ? OR (started_at = ? AND id < ?))`)
		args = append(args, at, at, q.AfterID)
	}
	clause := ""
	if len(where) > 0 {
		clause = `WHERE ` + strings.Join(where, ` AND `)
	}
	args = append(args, q.Limit)
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT id, user_id, device, channel_id, channel_name,
		programme, started_at, ended_at FROM viewings `+clause+`
		ORDER BY started_at DESC, id DESC LIMIT ?`), args...)
	if err != nil {
		return nil, fmt.Errorf("list viewings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Viewing, 0, q.Limit)
	for rows.Next() {
		var v Viewing
		var started, ended int64
		if err := rows.Scan(&v.ID, &v.UserID, &v.Device, &v.ChannelID, &v.ChannelName, &v.Programme,
			&started, &ended); err != nil {
			return nil, fmt.Errorf("scan viewing: %w", err)
		}
		v.StartedAt, v.EndedAt = time.Unix(started, 0).UTC(), time.Unix(ended, 0).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *sqlStore) DeleteViewings(ctx context.Context, userID string) (int, error) {
	query, args := `DELETE FROM viewings`, []any{}
	if userID != "" {
		query += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	result, err := s.db.ExecContext(ctx, s.ph(query), args...)
	if err != nil {
		return 0, fmt.Errorf("delete viewings: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete viewings: %w", err)
	}
	return int(n), nil
}
