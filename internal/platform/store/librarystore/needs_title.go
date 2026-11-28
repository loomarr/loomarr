package librarystore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

// A maintainer's answer is the title's, and a title may be reported by several origins (a movie in
// parts, a film's copies), any of which its row can take as its own. So a maintainer's write and
// delete act on every origin of the title in one transaction, rather than on one canonical key:
// an ignore and a link are read per origin, by the list and by the scanner.

// deleteTitleAnswers deletes the answers on origin and the title's other origins, only the ignores
// when ignoredOnly. Each is deleted by its key.
func (s *Store) deleteTitleAnswers(ctx context.Context, tx execer, origin LibraryOrigin, ignoredOnly bool) (sql.Result, error) {
	rows, err := tx.QueryContext(ctx, s.ph(`SELECT o2.authority_id, o2.external_item_id FROM inventory_item_origins o1
		CROSS JOIN inventory_item_origins o2
		WHERE o2.item_id = o1.item_id AND o1.authority_id = ? AND o1.external_item_id = ?`), origin.Authority, origin.Key)
	if err != nil {
		return nil, err
	}
	origins := []LibraryOrigin{origin}
	for rows.Next() {
		var o LibraryOrigin
		if err := rows.Scan(&o.Authority, &o.Key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if o != origin {
			origins = append(origins, o)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	query := `DELETE FROM library_answers WHERE origin_authority = ? AND origin_key = ?`
	if ignoredOnly {
		query += ` AND ignored = TRUE`
	}
	var total int64
	for _, o := range origins {
		res, err := tx.ExecContext(ctx, s.ph(query), o.Authority, o.Key)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		total += n
	}
	return driver.RowsAffected(total), nil
}

// spreadAnswer copies the answer on origin to the title's other origins that have none.
func (s *Store) spreadAnswer(ctx context.Context, tx execer, o LibraryOrigin) error {
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_answers
		(origin_authority, origin_key, kind, namespace, external_id, ignored, kept, answered_at_ms, answered_by)
		SELECT DISTINCT o2.authority_id, o2.external_item_id, a.kind, a.namespace, a.external_id, a.ignored, a.kept,
			a.answered_at_ms, a.answered_by
		FROM library_answers a
		JOIN inventory_item_origins o1 ON o1.authority_id = a.origin_authority AND o1.external_item_id = a.origin_key
		JOIN inventory_item_origins o2 ON o2.item_id = o1.item_id
		WHERE a.origin_authority = ? AND a.origin_key = ?
		ON CONFLICT (origin_authority, origin_key) DO NOTHING`), o.Authority, o.Key); err != nil {
		return fmt.Errorf("spread library answer: %w", err)
	}
	return nil
}

// replaceTitleAnswer clears the title's answers, saves one with insert, and copies it to the
// title's origins, all or nothing.
func (s *Store) replaceTitleAnswer(ctx context.Context, o LibraryOrigin, insert func(execer) (sql.Result, error)) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := s.deleteTitleAnswers(ctx, tx, o, false); err != nil {
		return err
	}
	if _, err := insert(tx); err != nil {
		return err
	}
	if err := s.spreadAnswer(ctx, tx, o); err != nil {
		return err
	}
	return tx.Commit()
}
