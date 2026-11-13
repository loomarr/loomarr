package channeldatastore

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

type FeedbackScope string

const (
	FeedbackHousehold FeedbackScope = "household"
	FeedbackChannel   FeedbackScope = "channel"
)

type FeedbackAction string

const (
	FeedbackKeep     FeedbackAction = "keep"
	FeedbackLess     FeedbackAction = "less"
	FeedbackNever    FeedbackAction = "never"
	FeedbackSurprise FeedbackAction = "surprise"
	FeedbackClear    FeedbackAction = "clear"
)

type DiscoveryFeedback struct {
	ID string
	// ActorID is the member who gave the feedback: "" for the API token, which is no member, and
	// once that member is deleted (actor_id is a foreign key to users, SET NULL, #2793).
	ActorID   string
	Scope     FeedbackScope
	ScopeID   string
	Target    provision.Key
	Action    FeedbackAction
	Reason    string
	CreatedAt time.Time
}

type FeedbackFilter struct {
	Scope   FeedbackScope
	ScopeID string
}

// PurgeSupersededDiscoveryFeedback deletes the choices EffectiveDiscoveryFeedback can never
// return: one with a newer choice about the same title in the same scope, then a clear with
// nothing older left to clear. What every reader sees is unchanged, and a household's rows are
// bounded by the titles it has an opinion about.
//
// The second statement only deletes a clear with no OLDER row, so a choice and its clear that
// both land between the two statements keep each other: the clear is not the oldest.
func (s *Store) PurgeSupersededDiscoveryFeedback(ctx context.Context) (int, error) {
	// scope_id is NULL for the household, and NULL = NULL is never true.
	sameTarget := `n.scope = discovery_feedback.scope AND n.scope_id IS NOT DISTINCT FROM discovery_feedback.scope_id
		AND n.target_key = discovery_feedback.target_key`
	superseded, err := s.h.ExecContext(ctx, `DELETE FROM discovery_feedback WHERE EXISTS (
		SELECT 1 FROM discovery_feedback n WHERE `+sameTarget+` AND (n.created_at_ns > discovery_feedback.created_at_ns
		   OR (n.created_at_ns = discovery_feedback.created_at_ns AND n.id > discovery_feedback.id)))`)
	if err != nil {
		return 0, fmt.Errorf("purge superseded discovery feedback: %w", err)
	}
	clears, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM discovery_feedback WHERE action = ? AND NOT EXISTS (
		SELECT 1 FROM discovery_feedback n WHERE `+sameTarget+` AND (n.created_at_ns < discovery_feedback.created_at_ns
		   OR (n.created_at_ns = discovery_feedback.created_at_ns AND n.id < discovery_feedback.id)))`),

		string(FeedbackClear))
	if err != nil {
		return 0, fmt.Errorf("purge spent discovery feedback clears: %w", err)
	}
	a, _ := superseded.RowsAffected()
	b, _ := clears.RowsAffected()
	return int(a + b), nil
}

// EffectiveDiscoveryFeedback resolves newest-event-wins within each scope and
// channel-over-household precedence. Inputs must be newest-first, as returned by
// ListDiscoveryFeedback. A channel clear removes only the override, allowing the
// effective household event to reappear.
func EffectiveDiscoveryFeedback(household, channel []DiscoveryFeedback) []DiscoveryFeedback {
	overrides := make(map[provision.Key]bool, len(channel))
	seenChannel := make(map[provision.Key]bool, len(channel))
	out := make([]DiscoveryFeedback, 0, len(channel)+len(household))
	for _, event := range channel {
		if seenChannel[event.Target] {
			continue
		}
		seenChannel[event.Target] = true
		if event.Action == FeedbackClear {
			continue
		}
		overrides[event.Target] = true
		out = append(out, event)
	}
	seenHousehold := make(map[provision.Key]bool, len(household))
	for _, event := range household {
		if seenHousehold[event.Target] {
			continue
		}
		seenHousehold[event.Target] = true
		if event.Action == FeedbackClear || overrides[event.Target] {
			continue
		}
		out = append(out, event)
	}
	return out
}

func (s *Store) AppendDiscoveryFeedback(ctx context.Context, feedback DiscoveryFeedback) error {
	if feedback.ID == "" || feedback.Target == "" {
		return fmt.Errorf("append discovery feedback: id and target are required")
	}
	if !validFeedbackScope(feedback.Scope, feedback.ScopeID) || !validFeedbackAction(feedback.Action) {
		return fmt.Errorf("append discovery feedback: invalid scope or action")
	}
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = time.Now().UTC()
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("append discovery feedback: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if feedback.Scope == FeedbackChannel {
		// This no-op UPDATE is the one portable ownership lock across the shared SQL path. Postgres
		// holds the Channel row against DELETE until commit; SQLite acquires its serialized writer
		// lock. DeleteChannel takes the reciprocal transaction, so either append commits first and
		// purge removes it, or purge commits first and this reports ErrNotFound.
		result, err := tx.ExecContext(ctx, s.h.Rebind(`UPDATE channels SET id = id WHERE id = ?`), feedback.ScopeID)
		if err != nil {
			return fmt.Errorf("append discovery feedback: lock channel: %w", err)
		}
		matched, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("append discovery feedback: lock channel affected rows: %w", err)
		}
		if matched == 0 {
			return storedb.ErrNotFound
		}
	}
	_, err = tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO discovery_feedback
		(id, actor_id, scope, scope_id, target_key, action, reason, created_at_ns)
		VALUES (?, (SELECT id FROM users WHERE id = ?), ?, ?, ?, ?, ?, ?)`),

		feedback.ID, feedback.ActorID, feedback.Scope,
		storedb.OptionalID(feedback.ScopeID), feedback.Target, feedback.Action, feedback.Reason, feedback.CreatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("append discovery feedback: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("append discovery feedback: commit: %w", err)
	}
	return nil
}

func (s *Store) ListDiscoveryFeedback(ctx context.Context, filter FeedbackFilter) ([]DiscoveryFeedback, error) {
	if !validFeedbackScope(filter.Scope, filter.ScopeID) {
		return nil, fmt.Errorf("list discovery feedback: invalid scope")
	}
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT id, actor_id, scope, scope_id, target_key,
		action, reason, created_at_ns FROM discovery_feedback WHERE scope = ? AND scope_id IS NOT DISTINCT FROM ?
		ORDER BY created_at_ns DESC, id DESC`),

		filter.Scope, storedb.OptionalID(filter.ScopeID))
	if err != nil {
		return nil, fmt.Errorf("list discovery feedback: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DiscoveryFeedback
	for rows.Next() {
		var feedback DiscoveryFeedback
		var createdAt int64
		if err := rows.Scan(&feedback.ID, (*storedb.OptionalID)(&feedback.ActorID), &feedback.Scope, (*storedb.OptionalID)(&feedback.ScopeID),
			&feedback.Target, &feedback.Action, &feedback.Reason, &createdAt); err != nil {
			return nil, fmt.Errorf("list discovery feedback: %w", err)
		}
		feedback.CreatedAt = time.Unix(0, createdAt).UTC()
		out = append(out, feedback)
	}
	return out, rows.Err()
}

func validFeedbackScope(scope FeedbackScope, scopeID string) bool {
	return (scope == FeedbackHousehold && scopeID == "") || (scope == FeedbackChannel && scopeID != "")
}

func validFeedbackAction(action FeedbackAction) bool {
	switch action {
	case FeedbackKeep, FeedbackLess, FeedbackNever, FeedbackSurprise, FeedbackClear:
		return true
	default:
		return false
	}
}
