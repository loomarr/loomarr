package channeldatastore

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) HideIdea(ctx context.Context, userID, ideaID string, at time.Time) error {
	if userID == "" || ideaID == "" || at.IsZero() {
		return fmt.Errorf("hide idea: user, idea and time are required")
	}
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(`INSERT INTO user_hidden_ideas (user_id, idea_id, hidden_at_ms)
		VALUES (?, ?, ?) ON CONFLICT (user_id, idea_id) DO NOTHING`),
		userID, ideaID, at.UnixMilli()); err != nil {
		return fmt.Errorf("hide idea: %w", err)
	}
	return nil
}

func (s *Store) UnhideIdea(ctx context.Context, userID, ideaID string) error {
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM user_hidden_ideas WHERE user_id = ? AND idea_id = ?`),
		userID, ideaID); err != nil {
		return fmt.Errorf("unhide idea: %w", err)
	}
	return nil
}

func (s *Store) HiddenIdeas(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT idea_id FROM user_hidden_ideas WHERE user_id = ?`), userID)
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
