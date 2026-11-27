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

func (s *Store) RestoreLibraryIgnored(ctx context.Context, itemIDs []string) ([]LibraryAnswered, error) {
	return s.answerEach(ctx, "restore ignored library titles", ErrNotIgnored, itemIDs,
		`SELECT i.origin_authority, i.origin_key FROM library_items i WHERE i.id = ?`,
		func(ctx context.Context, tx execer, o LibraryOrigin, _ string) (sql.Result, error) {
			return tx.ExecContext(ctx, s.ph(`DELETE FROM library_answers
				WHERE origin_authority = ? AND origin_key = ? AND ignored = TRUE`), o.Authority, o.Key)
		})
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
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
		if changed != 1 {
			return nil, refused
		}
		out = append(out, t)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return out, nil
}
