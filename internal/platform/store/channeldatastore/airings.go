package channeldatastore

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"github.com/loomarr/loomarr/internal/provision"
)

// ScheduledAiring is one programme a channel's schedule put on air (#1677).
type ScheduledAiring struct {
	Key           provision.Key
	LibraryItemID string
	AiredAt       time.Time
}

// Airing history (§5, programming-design §3.1) — what a channel actually broadcast.
//
// The scheduler's separation rules (§3) are WITHIN-CYCLE: they bound what recurs inside one pass
// of the deck, and when the deck wraps the memory resets. This is the only record of what played
// across cycles, and it exists so placement can prefer what has NOT been on recently.
//
// One row per airing of a UNIT (episode or film) per channel, with the instant the row was
// written — see migration 00125 for why the per-key upsert of 00019 was the wrong shape (#1674).

// airingRetention is how far back RecordAiring keeps every airing of a unit. Older rows are
// pruned down to the newest one, which is all a read as of any instant inside the retention
// horizon needs. Eight days covers a week-long rolling window plus the guide's 7-day forward span.
const airingRetention = 8 * 24 * time.Hour

// RecordAiring logs that a programme (one episode or film) started airing on a channel at
// `airedAt`, observed at `recordedAt`.
//
// ⚠ Called from PLAYOUT, when a programme is actually resolved for streaming — never from
// scheduling or reconcile. Those re-run on every sweep and would record SCHEDULED rather than
// AIRED, which would poison the very signal this feeds (a title would look "recently aired"
// because it was merely planned).
//
// Idempotent per airing: playout re-resolves the same programme many times, always with the same
// start, and only the first write lands, so `recorded_at` is when the airing was FIRST observed.
// That is what lets LastAiredByChannel ignore rows written after a window opened.
func (s *Store) RecordAiring(ctx context.Context, channelID string, key provision.Key, libraryItemID string, airedAt, recordedAt time.Time) error {
	return s.recordAiringIn(ctx, "airings", channelID, key, libraryItemID, airedAt, recordedAt)
}

// RecordScheduledAirings logs programmes a channel's schedule put on air, watched or not (#1677).
// It is the scheduled signal; RecordAiring is the watched one, and the two never share a table, so
// neither poisons the other. Each airing is written once, with the same retention as `airings`.
func (s *Store) RecordScheduledAirings(ctx context.Context, channelID string, airings []ScheduledAiring, recordedAt time.Time) error {
	for _, a := range airings {
		if err := s.recordAiringIn(ctx, "scheduled_airings", channelID, a.Key, a.LibraryItemID, a.AiredAt, recordedAt); err != nil {
			return err
		}
	}
	return nil
}

// recordAiringIn writes one airing to table (airings or scheduled_airings), once, and prunes the
// unit's rows past the retention horizon down to the newest of them.
func (s *Store) recordAiringIn(ctx context.Context, table, channelID string, key provision.Key, libraryItemID string, airedAt, recordedAt time.Time) error {
	if channelID == "" || libraryItemID == "" {
		return nil // nothing identifiable to record; not an error (playout must never fail on telemetry)
	}
	res, err := s.h.ExecContext(ctx, s.h.Rebind(
		`INSERT INTO `+table+` (channel_id, library_item_id, aired_at, recorded_at, key) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (channel_id, library_item_id, aired_at) DO NOTHING`),

		channelID, libraryItemID, storedb.Epoch(airedAt), storedb.Epoch(recordedAt), string(key))
	if err != nil {
		return fmt.Errorf("record %s %s/%s: %w", table, channelID, libraryItemID, err)
	}
	if n, nerr := res.RowsAffected(); nerr != nil || n == 0 {
		return nil // a repeat of an airing already logged: nothing new to prune
	}
	// Keep every airing inside the retention horizon plus the newest one older than it.
	horizon := storedb.Epoch(recordedAt.Add(-airingRetention))
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(
		`DELETE FROM `+table+` WHERE channel_id = ? AND library_item_id = ? AND recorded_at < ?
		   AND aired_at < (SELECT MAX(aired_at) FROM `+table+`
		                   WHERE channel_id = ? AND library_item_id = ? AND recorded_at < ?)`),

		channelID, libraryItemID, horizon, channelID, libraryItemID, horizon); err != nil {
		return fmt.Errorf("prune %s %s/%s: %w", table, channelID, libraryItemID, err)
	}
	return nil
}

// LastAiredByChannel returns, per unit (library item id), the latest airing on one channel whose
// row was recorded strictly before `before`.
//
// The cutoff is the start of the window being arranged, so everything a window's arrangement
// reads was already written when the window opened: a tune-in during the window, including one
// that records a programme which started before the boundary, cannot re-arrange it. A unit that
// has not aired (as of `before`) is absent — callers read absence as "never aired", which ranks
// it first. The whole channel at once, because that is how placement consumes it.
func (s *Store) LastAiredByChannel(ctx context.Context, channelID string, before time.Time) (map[string]time.Time, error) {
	return s.lastAiredIn(ctx, "airings", channelID, before)
}

// LastScheduledByChannel is LastAiredByChannel over the scheduled ledger (#1677).
func (s *Store) LastScheduledByChannel(ctx context.Context, channelID string, before time.Time) (map[string]time.Time, error) {
	return s.lastAiredIn(ctx, "scheduled_airings", channelID, before)
}

func (s *Store) lastAiredIn(ctx context.Context, table, channelID string, before time.Time) (map[string]time.Time, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT library_item_id, MAX(aired_at) FROM `+table+` WHERE channel_id = ? AND recorded_at < ?
		 GROUP BY library_item_id`),

		channelID, storedb.Epoch(before))
	if err != nil {
		return nil, fmt.Errorf("list %s %s: %w", table, channelID, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var airedAt int64
		if serr := rows.Scan(&id, &airedAt); serr != nil {
			return nil, fmt.Errorf("scan airing: %w", serr)
		}
		out[id] = time.Unix(airedAt, 0)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, fmt.Errorf("iterate airings %s: %w", channelID, rerr)
	}
	return out, nil
}

// LastAiredByKey returns, per provisioning key, the latest airing of ANY of its units on one
// channel. A series key covers many episodes, so this is "when did this title last air", the
// question a lineup entry asks. A key that has not aired inside the retained history is absent;
// callers read absence as "never aired".
func (s *Store) LastAiredByKey(ctx context.Context, channelID string) (map[provision.Key]time.Time, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT key, MAX(aired_at) FROM airings WHERE channel_id = ? GROUP BY key`),
		channelID)
	if err != nil {
		return nil, fmt.Errorf("list airings by key %s: %w", channelID, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[provision.Key]time.Time{}
	for rows.Next() {
		var key string
		var airedAt int64
		if serr := rows.Scan(&key, &airedAt); serr != nil {
			return nil, fmt.Errorf("scan airing by key: %w", serr)
		}
		out[provision.Key(key)] = time.Unix(airedAt, 0)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, fmt.Errorf("iterate airings by key %s: %w", channelID, rerr)
	}
	return out, nil
}
