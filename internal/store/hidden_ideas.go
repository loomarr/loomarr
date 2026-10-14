package store

import (
	"context"
	"fmt"
	"time"
)

// HiddenIdeaStore keeps the channel ideas each person hid on Home (#1665). Idea ids are the
// ideas package's stable ids ("genre:comedy"), so a hide outlives the library changing under it.
type HiddenIdeaStore interface {
	// HideIdea hides an idea for a person. Idempotent: a repeat keeps the first time.
	HideIdea(ctx context.Context, userID, ideaID string, at time.Time) error
	// UnhideIdea is the undo; unhiding an idea that isn't hidden is not an error.
	UnhideIdea(ctx context.Context, userID, ideaID string) error
	// HiddenIdeas is the set of a person's hidden idea ids.
	HiddenIdeas(ctx context.Context, userID string) (map[string]bool, error)
}

func (s *sqlStore) HideIdea(ctx context.Context, userID, ideaID string, at time.Time) error {
	if userID == "" || ideaID == "" || at.IsZero() {
		return fmt.Errorf("hide idea: user, idea and time are required")
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO user_hidden_ideas (user_id, idea_id, hidden_at)
		VALUES (?, ?, ?) ON CONFLICT (user_id, idea_id) DO NOTHING`), userID, ideaID, at.UnixMilli()); err != nil {
		return fmt.Errorf("hide idea: %w", err)
	}
	return nil
}

func (s *sqlStore) UnhideIdea(ctx context.Context, userID, ideaID string) error {
	if _, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM user_hidden_ideas WHERE user_id = ? AND idea_id = ?`),
		userID, ideaID); err != nil {
		return fmt.Errorf("unhide idea: %w", err)
	}
	return nil
}

func (s *sqlStore) HiddenIdeas(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT idea_id FROM user_hidden_ideas WHERE user_id = ?`), userID)
	if err != nil {
		return nil, fmt.Errorf("hidden ideas: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
