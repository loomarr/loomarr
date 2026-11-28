package librarystore

import (
	"context"
	"errors"
	"fmt"
)

// A link and an ignore are their origin's: each is written, read and taken back by its own key, so
// an answer for one file of a title joined from several never reaches the others. A home video is
// the only answer that is the title's. A title may be reported by several origins (a movie in
// parts, a film's copies), any of which its row can take as its own, so the keep is written on
// every origin the title has, and read on any origin of the title (titleKept, KeptLibraryTitle):
// a part or a copy that joins later is kept by that read, and the keep stands while any origin it
// was written on still reports the title. Every origin is used rather than one canonical key
// because answers are keyed and read by origin, through the primary key, and the scanner asks
// about a file before it knows the file's title, by the file's origin and groups: a key of the
// title's own would be one more join to keep indexed, and one more thing to move when titles join.
// Any other answer the maintainer gives or takes back on any origin of a kept title ends the keep
// on all of its origins, in the same transaction (clearTitleKept).

// titleOrigins are origin and the other origins of every title that reports it or holds it as
// its row's origin.
func (s *Store) titleOrigins(ctx context.Context, tx execer, origin LibraryOrigin) ([]LibraryOrigin, error) {
	rows, err := tx.QueryContext(ctx, s.ph(`WITH title AS (
			SELECT item_id AS id FROM inventory_item_origins WHERE authority_id = ? AND external_item_id = ?
			UNION SELECT id FROM library_items WHERE origin_authority = ? AND origin_key = ?)
		SELECT o.authority_id, o.external_item_id FROM title t CROSS JOIN inventory_item_origins o WHERE o.item_id = t.id
		UNION SELECT i.origin_authority, i.origin_key FROM title t CROSS JOIN library_items i
			WHERE i.id = t.id AND i.origin_key <> ''`), origin.Authority, origin.Key, origin.Authority, origin.Key)
	if err != nil {
		return nil, err
	}
	out := []LibraryOrigin{origin}
	for rows.Next() {
		var o LibraryOrigin
		if err := rows.Scan(&o.Authority, &o.Key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if o != origin {
			out = append(out, o)
		}
	}
	return out, errors.Join(rows.Err(), rows.Close())
}

// clearTitleKept ends the keep of the title origin belongs to, on each of its origins by its key.
// Links and ignores are left as they are.
func (s *Store) clearTitleKept(ctx context.Context, tx execer, origin LibraryOrigin) error {
	origins, err := s.titleOrigins(ctx, tx, origin)
	if err != nil {
		return fmt.Errorf("clear kept title: %w", err)
	}
	for _, o := range origins {
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_answers
			WHERE origin_authority = ? AND origin_key = ? AND kept = TRUE`), o.Authority, o.Key); err != nil {
			return fmt.Errorf("clear kept title: %w", err)
		}
	}
	return nil
}

// keepTitle writes the keep on origin, then on the title's other origins that have no answer. The
// result is origin's write.
func (s *Store) keepTitle(ctx context.Context, tx execer, origin LibraryOrigin, kind string, at int64) (int64, error) {
	origins, err := s.titleOrigins(ctx, tx, origin)
	if err != nil {
		return 0, err
	}
	var first int64
	for i, o := range origins {
		res, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_answers
			(origin_authority, origin_key, kind, kept, answered_at_ms, answered_by) VALUES (?, ?, ?, TRUE, ?, ?)
			ON CONFLICT (origin_authority, origin_key) DO NOTHING`), o.Authority, o.Key, kind, at, string(AnsweredByYou))
		if err != nil {
			return 0, err
		}
		if i == 0 {
			if first, err = res.RowsAffected(); err != nil {
				return 0, err
			}
		}
	}
	return first, nil
}

// keptQuery is the query KeptLibraryTitle runs, and its arguments: whether origin, or the title
// that reports origin or holds one of the group ids, is kept. Each title's origins are read first
// (CROSS JOIN keeps that order on SQLite), so every answer is found by its primary key.
func keptQuery(kind string, origin LibraryOrigin, groups []LibraryID) (string, []any) {
	items := `SELECT item_id FROM inventory_item_origins WHERE authority_id = ? AND external_item_id = ?`
	args := []any{origin.Authority, origin.Key, kind, origin.Authority, origin.Key}
	for _, g := range groups {
		items += ` UNION SELECT item_id FROM inventory_external_ids WHERE namespace = ? AND external_id = ?`
		args = append(args, g.Namespace, g.Value)
	}
	args = append(args, kind)
	return `SELECT EXISTS (SELECT 1 FROM library_answers WHERE origin_authority = ? AND origin_key = ?
				AND kept = TRUE AND kind = ?)
			OR EXISTS (SELECT 1 FROM (` + items + `) t CROSS JOIN inventory_item_origins o CROSS JOIN library_answers a
				WHERE o.item_id = t.item_id AND a.origin_authority = o.authority_id
					AND a.origin_key = o.external_item_id AND a.kept = TRUE AND a.kind = ?)`, args
}
