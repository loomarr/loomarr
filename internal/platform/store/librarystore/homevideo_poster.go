package librarystore

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// LibraryHomeVideoFrame is where a home video's picture comes from: the main file of a kept
// movie or show.
type LibraryHomeVideoFrame struct {
	ItemID     string
	Path       string
	DurationMs int64
	// HDR says the file's picture is PQ or HLG, so a frame of it is tone-mapped.
	HDR bool
	// Revision is the inventory's revision of the file: a replaced file has another one.
	Revision string
}

// file identifies the file a frame is taken from. A poster ref carries it, and "" marks a ref
// that is no frame of the title's own file.
func (f LibraryHomeVideoFrame) file() string { return f.Revision + " " + f.Path }

// HomeVideosWithoutPoster returns each kept title that has no poster ref and has a file of known
// length, with its main file: a movie's first playing part, or a show's first playing episode
// file by path. In item id order.
func (s *Store) HomeVideosWithoutPoster(ctx context.Context) ([]LibraryHomeVideoFrame, error) {
	return s.homeVideoFrames(ctx, s.h, ` AND NOT EXISTS (SELECT 1 FROM library_item_images r
		WHERE r.item_id = i.id AND r.role = 'poster')`)
}

// homeVideoFrames returns the main file of each kept title that also matches also, a condition on
// the title i, whose placeholders take args.
func (s *Store) homeVideoFrames(ctx context.Context, q storedb.Querier, also string, args ...any) ([]LibraryHomeVideoFrame, error) {
	hdr := `EXISTS (SELECT 1 FROM library_streams v WHERE v.item_id = s.item_id AND v.path = s.path
		AND v.color_transfer IN ('smpte2084', 'arib-std-b67'))`
	wanted := titleKept("i") + also + ` AND s.plays AND s.duration_ms > 0`
	rows, err := q.QueryContext(ctx, s.ph(`SELECT i.id AS id, s.path AS path, s.duration_ms, `+hdr+`, s.revision, s.part AS part
			FROM library_items i JOIN library_sources s ON s.item_id = i.id
			WHERE i.kind = 'movie' AND `+wanted+`
		UNION ALL SELECT i.id, s.path, s.duration_ms, `+hdr+`, s.revision, s.part
			FROM library_items i JOIN library_files f ON f.inventory_series_id = i.id AND `+presentFile+`
			JOIN library_sources s ON s.item_id = f.inventory_item_id AND s.path = f.path
			WHERE i.kind = 'series' AND `+wanted+`
		ORDER BY id, path, part`), append(append([]any{}, args...), args...)...)
	if err != nil {
		return nil, err
	}
	var out []LibraryHomeVideoFrame
	part := 0
	for rows.Next() {
		var f LibraryHomeVideoFrame
		var p int
		if err := rows.Scan(&f.ItemID, &f.Path, &f.DurationMs, &f.HDR, &f.Revision, &p); err != nil {
			_ = rows.Close()
			return nil, err
		}
		switch n := len(out); {
		case n == 0 || out[n-1].ItemID != f.ItemID:
			out, part = append(out, f), p
		case p < part:
			// A movie's parts play in part order, whatever their names.
			out[n-1], part = f, p
		}
	}
	return out, errors.Join(rows.Err(), rows.Close())
}

// PutHomeVideoPoster adds frame's poster ref if the title is still kept, has no poster and still
// plays frame's file.
func (s *Store) PutHomeVideoPoster(ctx context.Context, frame LibraryHomeVideoFrame, imageHash string) (bool, error) {
	put := false
	err := s.withLockedItem(ctx, frame.ItemID, func(tx storedb.Tx) error {
		now, err := s.homeVideoFrames(ctx, tx, ` AND i.id = ? AND NOT EXISTS (SELECT 1 FROM library_item_images r
			WHERE r.item_id = i.id AND r.role = 'poster')`, frame.ItemID)
		if err != nil || len(now) == 0 || now[0].file() != frame.file() {
			return err
		}
		_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO library_item_images (item_id, role, image_hash, source)
			VALUES (?, 'poster', ?, ?)`), frame.ItemID, imageHash, frame.file())
		put = err == nil
		return err
	})
	if err != nil {
		return false, fmt.Errorf("put home video poster %s: %w", frame.ItemID, err)
	}
	return put, nil
}

// SetLibraryItemImage replaces the title's images in role with imageHash.
func (s *Store) SetLibraryItemImage(ctx context.Context, itemID, role, imageHash string) error {
	err := s.withLockedItem(ctx, itemID, func(tx storedb.Tx) error {
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_item_images WHERE item_id = ? AND role = ?`),
			itemID, role); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_item_images (item_id, role, image_hash)
			VALUES (?, ?, ?)`), itemID, role, imageHash)
		return err
	})
	if err != nil {
		return fmt.Errorf("set library %s %s: %w", role, itemID, err)
	}
	return nil
}

// ClearLibraryPoster removes the title's posters other than a frame of the file it still plays.
func (s *Store) ClearLibraryPoster(ctx context.Context, itemID string) error {
	err := s.withLockedItem(ctx, itemID, func(tx storedb.Tx) error {
		now, err := s.homeVideoFrames(ctx, tx, ` AND i.id = ?`, itemID)
		if err != nil {
			return err
		}
		keep := ""
		if len(now) > 0 {
			keep = now[0].file()
		}
		_, err = tx.ExecContext(ctx, s.ph(`DELETE FROM library_item_images
			WHERE item_id = ? AND role = 'poster' AND (source = '' OR source <> ?)`), itemID, keep)
		return err
	})
	if err != nil {
		return fmt.Errorf("clear library poster %s: %w", itemID, err)
	}
	return nil
}

// withLockedItem runs fn in a transaction that holds the title's row, so one writer of its images
// at a time reads and writes them: SQLite serialises writers, and the update takes Postgres's
// row lock.
func (s *Store) withLockedItem(ctx context.Context, itemID string, fn func(storedb.Tx) error) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_items SET kind = kind WHERE id = ?`), itemID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
