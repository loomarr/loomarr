package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Activity is one row in the Dashboard's Recent activity feed (§5, §12, V32).
//
// The feed answers "what has Loomarr been doing?", which is a different question from any
// per-entity state a GET already exposes — so it is its own table rather than a view over
// titles/channels/proposals.
type Activity struct {
	ID   string `json:"id"`
	At   int64  `json:"at"`   // unix seconds
	Kind string `json:"kind"` // title|channel|proposal|filler|user|system
	// Level drives the UI's coloured dot. Bounded on the way IN (see RecordActivity) so the
	// frontend can never receive a value it has no rendering for.
	Level     string `json:"level"` // info|warn|error
	Text      string `json:"text"`
	SubjectID string `json:"subjectId,omitempty"`
}

// Activity levels. A closed set: the UI maps each to a colour, and an unrecognised one would
// render as nothing at all.
const (
	ActivityInfo  = "info"
	ActivityWarn  = "warn"
	ActivityError = "error"
)

// ActivityKind values — the domain that wrote the row.
const (
	ActivityKindTitle    = "title"
	ActivityKindChannel  = "channel"
	ActivityKindProposal = "proposal"
	ActivityKindFiller   = "filler"
	ActivityKindUser     = "user"
	ActivityKindSystem   = "system"
)

// RecordActivity appends one feed row (§5, §12).
//
// ⚠ **Best-effort by contract, exactly like RecordAiring.** Callers write from inside a real
// operation — a title landing, a channel reconciling — and an error here must never propagate
// far enough to undo it. Recording that something happened cannot be allowed to stop it
// happening, so callers log and continue; this returns an error only so they CAN log it.
//
// An empty text is dropped rather than stored: a feed row nobody can read is worse than an
// absent one, because it occupies a slot in a list the operator is scanning.
func (s *sqlStore) RecordActivity(ctx context.Context, a Activity) error {
	if a.Text == "" {
		return nil
	}
	if a.ID == "" {
		a.ID = newActivityID()
	}
	if a.At == 0 {
		a.At = time.Now().Unix()
	}
	// ⚠ Normalised HERE, not trusted from the caller. Every write point is in-process today,
	// but a level the UI has no colour for renders as an invisible dot — a silent failure in
	// the one place whose job is to be legible.
	switch a.Level {
	case ActivityInfo, ActivityWarn, ActivityError:
	default:
		a.Level = ActivityInfo
	}

	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO activity (id, at, kind, level, text, subject_id) VALUES (?, ?, ?, ?, ?, ?)`),
		a.ID, a.At, a.Kind, a.Level, a.Text, a.SubjectID)
	if err != nil {
		return fmt.Errorf("record activity: %w", err)
	}
	return nil
}

// ListActivity returns the newest rows first, capped at limit.
//
// Newest-first with a limit is the ONLY read (§7). The feed is a glance at the top of a
// dashboard, not a query surface — filtering and paging belong to a log viewer, which is a
// different feature (§20).
func (s *sqlStore) ListActivity(ctx context.Context, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 20
	}
	// A caller asking for everything would turn a dashboard panel into a full-table scan on
	// the one table that grows without bound.
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT id, at, kind, level, text, subject_id FROM activity ORDER BY at DESC, id DESC LIMIT ?`), limit)
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Activity, 0, limit)
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.At, &a.Kind, &a.Level, &a.Text, &a.SubjectID); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PurgeActivity deletes rows older than `before` and returns how many went (§18.1).
//
// The feed is the one append-only table here, so it is the one that needs this. `airings`
// cannot outgrow channels × lineup-size; this grows with every event forever.
func (s *sqlStore) PurgeActivity(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM activity WHERE at < ?`), before.Unix())
	if err != nil {
		return 0, fmt.Errorf("purge activity: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		// The delete succeeded; only the count is unavailable. Reporting an error here would
		// make the job log a failure for work that was actually done.
		return 0, nil
	}
	return int(n), nil
}

// newActivityID mints a random row id. Random rather than sequential because the feed's
// ordering is `at`, and a monotonic id would imply an ordering guarantee across replicas that
// nothing enforces.
func newActivityID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Vanishingly unlikely; a timestamp id still yields a usable unique-enough row rather
		// than dropping the entry.
		return fmt.Sprintf("act_%d", time.Now().UnixNano())
	}
	return "act_" + hex.EncodeToString(b[:])
}
