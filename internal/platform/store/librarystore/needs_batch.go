package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) IgnoreLibraryNeeds(ctx context.Context, itemIDs []string, at time.Time) ([]LibraryAnswered, error) {
	// An answer saved since the read, even by the Matcher, leaves the insert with no row.
	return s.answerEach(ctx, "ignore library needs", ErrNotNeeded, itemIDs,
		`SELECT i.origin_authority, i.origin_key FROM library_items i WHERE i.id = ? AND `+needWhere+` AND `+itemListed,
		func(ctx context.Context, tx execer, o LibraryOrigin, itemID string) (sql.Result, error) {
			return tx.ExecContext(ctx, s.ph(`INSERT INTO library_answers
				(origin_authority, origin_key, kind, namespace, external_id, ignored, answered_at_ms, answered_by)
				SELECT ?, ?, kind, '', '', TRUE, ?, ? FROM library_items WHERE id = ?
				ON CONFLICT (origin_authority, origin_key) DO NOTHING`),
				o.Authority, o.Key, at.UnixMilli(), string(AnsweredByYou), itemID)
		})
}

func (s *Store) KeepLibraryNeed(ctx context.Context, itemID string, at time.Time) (LibraryAnswered, error) {
	out, err := s.answerEach(ctx, "keep library need", ErrNotKeepable, []string{itemID},
		`SELECT i.origin_authority, i.origin_key FROM library_items i WHERE i.id = ? AND `+needWhere+` AND `+itemListed+`
			AND NOT EXISTS (SELECT 1 FROM library_candidates c WHERE c.inventory_item_id = i.id)`,
		func(ctx context.Context, tx execer, o LibraryOrigin, itemID string) (sql.Result, error) {
			return tx.ExecContext(ctx, s.ph(`INSERT INTO library_answers
				(origin_authority, origin_key, kind, kept, answered_at_ms, answered_by)
				SELECT ?, ?, kind, TRUE, ?, ? FROM library_items WHERE id = ?
				ON CONFLICT (origin_authority, origin_key) DO NOTHING`),
				o.Authority, o.Key, at.UnixMilli(), string(AnsweredByYou), itemID)
		})
	if err != nil {
		return LibraryAnswered{}, err
	}
	return out[0], nil
}

// KeptLibraryTitle reports whether a file belongs to a title kept as a home video: the answer is
// for origin itself, or for the title that reports origin or holds one of the group ids.
func (s *Store) KeptLibraryTitle(ctx context.Context, kind string, origin LibraryOrigin, groups []LibraryID) (bool, error) {
	items := `SELECT item_id FROM inventory_item_origins WHERE authority_id = ? AND external_item_id = ?`
	args := []any{kind, origin.Authority, origin.Key, origin.Authority, origin.Key}
	for _, g := range groups {
		items += ` UNION SELECT item_id FROM inventory_external_ids WHERE namespace = ? AND external_id = ?`
		args = append(args, g.Namespace, g.Value)
	}
	var n int
	if err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM library_answers
		WHERE kept = TRUE AND kind = ? AND (origin_authority, origin_key) IN (SELECT CAST(? AS TEXT), CAST(? AS TEXT)
			UNION SELECT authority_id, external_item_id FROM inventory_item_origins WHERE item_id IN (`+items+`))`),
		args...).Scan(&n); err != nil {
		return false, fmt.Errorf("kept library title: %w", err)
	}
	return n > 0, nil
}

func (s *Store) RestoreLibraryIgnored(ctx context.Context, itemIDs []string) ([]LibraryAnswered, error) {
	return s.answerEach(ctx, "restore ignored library titles", ErrNotIgnored, itemIDs,
		`SELECT i.origin_authority, i.origin_key FROM library_items i WHERE i.id = ?`,
		func(ctx context.Context, tx execer, o LibraryOrigin, _ string) (sql.Result, error) {
			return s.deleteTitleAnswers(ctx, tx, o, true)
		})
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// answerEach reads each title's origin and writes its answer in one transaction. A title find
// does not return, or one write changes nothing for, is refused, and nothing is kept.
func (s *Store) answerEach(ctx context.Context, what string, refused error, itemIDs []string, find string,
	write func(context.Context, execer, LibraryOrigin, string) (sql.Result, error),
) ([]LibraryAnswered, error) {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer func() { _ = tx.Rollback() }()
	out := make([]LibraryAnswered, 0, len(itemIDs))
	for _, id := range itemIDs {
		t := LibraryAnswered{ID: id}
		err := tx.QueryRowContext(ctx, s.ph(find), id).Scan(&t.Origin.Authority, &t.Origin.Key)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, refused
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		res, err := write(ctx, tx, t.Origin, id)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		if changed == 0 {
			return nil, refused
		}
		// A new answer goes to the title's other origins too; a restore left none to copy.
		if err := s.spreadAnswer(ctx, tx, t.Origin); err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		out = append(out, t)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return out, nil
}
