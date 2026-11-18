package clipstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FillerBreak is one assembled break as the store keeps it: the pod is an opaque JSON document the
// filler domain owns. Start is the break's start on its channel's timeline, which with the channel
// identifies it (#3838 step 2).
type FillerBreak struct {
	ChannelID string
	Start     time.Time
	// Ceiling is the slot length the pod was assembled to; Length is the media it holds.
	Ceiling, Length time.Duration
	PodJSON         string
	PlannedAt       time.Time
}

// SaveFillerBreaks stores assembled breaks, replacing any already stored at the same channel and
// start, in one transaction.
func (s *Store) SaveFillerBreaks(ctx context.Context, breaks []FillerBreak) error {
	if len(breaks) == 0 {
		return nil
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("save filler breaks: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, b := range breaks {
		if _, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO filler_breaks
			(channel_id, start_ms, ceiling_ms, length_ms, pod_json, planned_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(channel_id, start_ms) DO UPDATE SET
			 ceiling_ms = excluded.ceiling_ms, length_ms = excluded.length_ms,
			 pod_json = excluded.pod_json, planned_at = excluded.planned_at`),
			b.ChannelID, b.Start.UnixMilli(), b.Ceiling.Milliseconds(), b.Length.Milliseconds(),
			b.PodJSON, epoch(b.PlannedAt)); err != nil {
			return fmt.Errorf("save filler break %s@%d: %w", b.ChannelID, b.Start.UnixMilli(), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save filler breaks: commit: %w", err)
	}
	return nil
}

// ListFillerBreaks returns a channel's stored breaks starting in [from, to], earliest first.
func (s *Store) ListFillerBreaks(ctx context.Context, channelID string, from, to time.Time) ([]FillerBreak, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT start_ms, ceiling_ms, length_ms, pod_json, planned_at
		FROM filler_breaks WHERE channel_id = ? AND start_ms >= ? AND start_ms <= ? ORDER BY start_ms`),
		channelID, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("list filler breaks %s: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []FillerBreak
	for rows.Next() {
		b := FillerBreak{ChannelID: channelID}
		var startMs, ceilingMs, lengthMs, planned int64
		if err := rows.Scan(&startMs, &ceilingMs, &lengthMs, &b.PodJSON, &planned); err != nil {
			return nil, fmt.Errorf("scan filler break: %w", err)
		}
		b.Start = time.UnixMilli(startMs).UTC()
		b.Ceiling = time.Duration(ceilingMs) * time.Millisecond
		b.Length = time.Duration(lengthMs) * time.Millisecond
		b.PlannedAt = fromEpoch(planned)
		out = append(out, b)
	}
	return out, rows.Err()
}

// KeepFillerBreaks deletes a channel's stored breaks other than the ones starting at `starts`:
// a break whose slot moved or left the window has nothing to air at its old start.
func (s *Store) KeepFillerBreaks(ctx context.Context, channelID string, starts []time.Time) error {
	if len(starts) == 0 {
		if _, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM filler_breaks WHERE channel_id = ?`), channelID); err != nil {
			return fmt.Errorf("clear filler breaks %s: %w", channelID, err)
		}
		return nil
	}
	// The kept starts are one channel's window of breaks, a few hundred at most, so they bind
	// directly; chunked at 500 to stay under every driver's placeholder limit.
	keep := make(map[int64]bool, len(starts))
	for _, t := range starts {
		keep[t.UnixMilli()] = true
	}
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT start_ms FROM filler_breaks WHERE channel_id = ?`), channelID)
	if err != nil {
		return fmt.Errorf("list filler breaks %s: %w", channelID, err)
	}
	var drop []int64
	for rows.Next() {
		var ms int64
		if err := rows.Scan(&ms); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan filler break start: %w", err)
		}
		if !keep[ms] {
			drop = append(drop, ms)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for len(drop) > 0 {
		n := min(len(drop), 500)
		args := []any{channelID}
		marks := make([]string, n)
		for i, ms := range drop[:n] {
			args = append(args, ms)
			marks[i] = "?"
		}
		if _, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM filler_breaks WHERE channel_id = ? AND start_ms IN (`+
			strings.Join(marks, ",")+`)`), args...); err != nil {
			return fmt.Errorf("delete filler breaks %s: %w", channelID, err)
		}
		drop = drop[n:]
	}
	return nil
}
