package store

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// Airing history (§5, programming-design §3.1) — what a channel actually broadcast.
//
// The scheduler's separation rules (§3) are WITHIN-CYCLE: they bound what recurs inside one pass
// of the deck, and when the deck wraps the memory resets. This is the only record of what played
// across cycles, and it exists so placement can prefer what has NOT been on recently.
//
// One row per (channel, key), holding the LAST airing — see migration 00019 for why a log would
// be the wrong shape.

// RecordAiring stamps that a programme aired on a channel.
//
// ⚠ Called from PLAYOUT, when a programme is actually resolved for streaming — never from
// scheduling or reconcile. Those re-run on every sweep and would record SCHEDULED rather than
// AIRED, which would poison the very signal this feeds (a title would look "recently aired"
// because it was merely planned).
//
// Upsert, not insert: the row IS the last airing. Re-airing the same programme moves the
// timestamp forward rather than accumulating history nobody reads.
func (s *sqlStore) RecordAiring(ctx context.Context, channelID string, key provision.Key, libraryItemID string, at time.Time) error {
	if channelID == "" || key == "" {
		return nil // nothing identifiable to record; not an error (playout must never fail on telemetry)
	}
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO airings (channel_id, key, library_item_id, aired_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(channel_id, key) DO UPDATE SET library_item_id = excluded.library_item_id, aired_at = excluded.aired_at`),
		channelID, string(key), libraryItemID, epoch(at))
	if err != nil {
		return fmt.Errorf("record airing %s/%s: %w", channelID, key, err)
	}
	return nil
}

// LastAiredByChannel returns the most recent airing per key on one channel.
//
// The whole channel at once, because that is how placement consumes it — one read per cycle
// computation rather than a lookup per candidate. A key that has never aired is ABSENT from the
// map; callers read absence as "never aired", which ranks it first.
func (s *sqlStore) LastAiredByChannel(ctx context.Context, channelID string) (map[provision.Key]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT key, aired_at FROM airings WHERE channel_id = ?`), channelID)
	if err != nil {
		return nil, fmt.Errorf("list airings %s: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[provision.Key]time.Time{}
	for rows.Next() {
		var key string
		var airedAt int64
		if serr := rows.Scan(&key, &airedAt); serr != nil {
			return nil, fmt.Errorf("scan airing: %w", serr)
		}
		out[provision.Key(key)] = time.Unix(airedAt, 0)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, fmt.Errorf("iterate airings %s: %w", channelID, rerr)
	}
	return out, nil
}
