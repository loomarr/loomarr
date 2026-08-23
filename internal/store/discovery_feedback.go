package store

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
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
	ID        string
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

type DiscoveryFeedbackStore interface {
	AppendDiscoveryFeedback(context.Context, DiscoveryFeedback) error
	ListDiscoveryFeedback(context.Context, FeedbackFilter) ([]DiscoveryFeedback, error)
}

func (s *sqlStore) AppendDiscoveryFeedback(ctx context.Context, feedback DiscoveryFeedback) error {
	if feedback.ID == "" || feedback.ActorID == "" || feedback.Target == "" {
		return fmt.Errorf("append discovery feedback: id, actor, and target are required")
	}
	if !validFeedbackScope(feedback.Scope, feedback.ScopeID) || !validFeedbackAction(feedback.Action) {
		return fmt.Errorf("append discovery feedback: invalid scope or action")
	}
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO discovery_feedback
		(id, actor_id, scope, scope_id, target_key, action, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), feedback.ID, feedback.ActorID, feedback.Scope,
		feedback.ScopeID, feedback.Target, feedback.Action, feedback.Reason, feedback.CreatedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("append discovery feedback: %w", err)
	}
	return nil
}

func (s *sqlStore) ListDiscoveryFeedback(ctx context.Context, filter FeedbackFilter) ([]DiscoveryFeedback, error) {
	if !validFeedbackScope(filter.Scope, filter.ScopeID) {
		return nil, fmt.Errorf("list discovery feedback: invalid scope")
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT id, actor_id, scope, scope_id, target_key,
		action, reason, created_at FROM discovery_feedback WHERE scope = ? AND scope_id = ?
		ORDER BY created_at DESC, id DESC`), filter.Scope, filter.ScopeID)
	if err != nil {
		return nil, fmt.Errorf("list discovery feedback: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DiscoveryFeedback
	for rows.Next() {
		var feedback DiscoveryFeedback
		var createdAt int64
		if err := rows.Scan(&feedback.ID, &feedback.ActorID, &feedback.Scope, &feedback.ScopeID,
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
