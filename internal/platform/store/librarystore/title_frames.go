package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// A title with no catalog artwork gets a frame of its main file (library.md "A home video's
// picture" and "A frame on a Needs you card"): a kept title as its poster, and a title waiting in
// Needs you under the frame ref role, which only Needs you reads.

// LibraryFrameRole is the ref role of a Needs you card's frame. The scanner never writes it.
const LibraryFrameRole = "frame"

// LibraryFrameFile is where a title's frame comes from: the main file of a kept movie or show, or
// of one waiting in Needs you.
type LibraryFrameFile struct {
	ItemID     string
	Path       string
	DurationMs int64
	// HDR says the file's picture is PQ or HLG, so a frame of it is tone-mapped.
	HDR bool
	// Revision is the inventory's revision of the file: a replaced file has another one.
	Revision string
}

// file identifies the file a frame is taken from. A poster or frame ref carries it, and "" marks a
// ref that is no frame of the title's own file.
func (f LibraryFrameFile) file() string { return f.Revision + " " + f.Path }

// waitingTitle is the condition on a title i that it waits in Needs you.
func waitingTitle() string {
	return `(` + needWhere + ` AND NOT ` + titleKept("i") + ` AND ` + itemListed + `)`
}

// noRef is the condition on a title i that it has no ref in role.
func noRef(role string) string {
	return ` AND NOT EXISTS (SELECT 1 FROM library_item_images r WHERE r.item_id = i.id AND r.role = '` + role + `')`
}

// HomeVideosWithoutPoster returns each kept title that has no poster ref and has a file of known
// length, with its main file: a movie's first playing part, or a show's first playing episode
// file by path. In item id order.
func (s *Store) HomeVideosWithoutPoster(ctx context.Context) ([]LibraryFrameFile, error) {
	return s.mainFiles(ctx, s.h, titleKept("i"), noRef("poster"))
}

// LibraryNeedsWithoutFrame returns each title waiting in Needs you that has no frame ref and has a
// file of known length that plays, with its main file, chosen as HomeVideosWithoutPoster chooses
// it. In item id order.
func (s *Store) LibraryNeedsWithoutFrame(ctx context.Context) ([]LibraryFrameFile, error) {
	return s.mainFiles(ctx, s.h, waitingTitle(), noRef(LibraryFrameRole))
}

// mainFiles returns the main file of each title that matches which and also, conditions on the
// title i; also's placeholders take args.
func (s *Store) mainFiles(ctx context.Context, q storedb.Querier, which, also string, args ...any) ([]LibraryFrameFile, error) {
	hdr := `EXISTS (SELECT 1 FROM library_streams v WHERE v.item_id = s.item_id AND v.path = s.path
		AND v.color_transfer IN ('smpte2084', 'arib-std-b67'))`
	wanted := which + also + ` AND s.plays AND s.duration_ms > 0`
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
	var out []LibraryFrameFile
	part := 0
	for rows.Next() {
		var f LibraryFrameFile
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
func (s *Store) PutHomeVideoPoster(ctx context.Context, frame LibraryFrameFile, imageHash string) (bool, error) {
	put, err := s.putFrame(ctx, titleKept("i"), "poster", frame, imageHash)
	if err != nil {
		return false, fmt.Errorf("put home video poster %s: %w", frame.ItemID, err)
	}
	return put, nil
}

// PutLibraryNeedFrame adds frame's frame ref if the title still waits in Needs you, has no frame
// and still plays frame's file.
func (s *Store) PutLibraryNeedFrame(ctx context.Context, frame LibraryFrameFile, imageHash string) (bool, error) {
	put, err := s.putFrame(ctx, waitingTitle(), LibraryFrameRole, frame, imageHash)
	if err != nil {
		return false, fmt.Errorf("put library need frame %s: %w", frame.ItemID, err)
	}
	return put, nil
}

// putFrame adds frame's ref in role if the title still matches which, has no ref in role and plays
// frame's file, read and written under the title's lock.
func (s *Store) putFrame(ctx context.Context, which, role string, frame LibraryFrameFile, imageHash string) (bool, error) {
	put := false
	err := s.withLockedItem(ctx, frame.ItemID, func(tx storedb.Tx) error {
		now, err := s.mainFiles(ctx, tx, which, ` AND i.id = ?`+noRef(role), frame.ItemID)
		if err != nil || len(now) == 0 || now[0].file() != frame.file() {
			return err
		}
		_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO library_item_images (item_id, role, image_hash, source)
			VALUES (?, ?, ?, ?)`), frame.ItemID, role, imageHash, frame.file())
		put = err == nil
		return err
	})
	return put, err
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
	if err := s.clearFrames(ctx, itemID, titleKept("i"), "poster"); err != nil {
		return fmt.Errorf("clear library poster %s: %w", itemID, err)
	}
	return nil
}

// clearFrames removes the title's refs in role other than a frame of the main file it plays while
// it matches which, read and written under the title's lock.
func (s *Store) clearFrames(ctx context.Context, itemID, which, role string) error {
	return s.withLockedItem(ctx, itemID, func(tx storedb.Tx) error {
		now, err := s.mainFiles(ctx, tx, which, ` AND i.id = ?`, itemID)
		if err != nil {
			return err
		}
		keep := ""
		if len(now) > 0 {
			keep = now[0].file()
		}
		_, err = tx.ExecContext(ctx, s.ph(`DELETE FROM library_item_images
			WHERE item_id = ? AND role = ? AND (source = '' OR source <> ?)`), itemID, role, keep)
		return err
	})
}

// ClearStaleLibraryFrames removes each home video's frame whose title plays another main file, and
// each Needs you frame whose title plays another main file or no longer waits in Needs you. A kept
// title with no main file keeps its frame: it has nothing to take another from.
func (s *Store) ClearStaleLibraryFrames(ctx context.Context) error {
	frames, err := s.mainFiles(ctx, s.h, titleKept("i"), ` AND EXISTS (SELECT 1 FROM library_item_images r
		WHERE r.item_id = i.id AND r.role = 'poster' AND r.source <> '')`)
	if err != nil {
		return fmt.Errorf("clear stale library frames: %w", err)
	}
	for _, f := range frames {
		var stale int
		if err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM library_item_images
			WHERE item_id = ? AND role = 'poster' AND source <> '' AND source <> ?`), f.ItemID, f.file()).Scan(&stale); err != nil {
			return fmt.Errorf("clear stale library frames: %w", err)
		}
		if stale == 0 {
			continue
		}
		// It reads the main file again under the title's lock.
		if err := s.ClearLibraryPoster(ctx, f.ItemID); err != nil {
			return err
		}
	}
	return s.clearStaleNeedFrames(ctx)
}

// clearStaleNeedFrames removes each frame ref that is not of the main file its title plays while
// it waits in Needs you.
func (s *Store) clearStaleNeedFrames(ctx context.Context) error {
	refs := map[string]string{}
	// It reads the titles and looks each one's frame up by its key; frames have no index by role.
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT i.id, (SELECT MIN(r.source) FROM library_item_images r
		WHERE r.item_id = i.id AND r.role = '`+LibraryFrameRole+`') AS source FROM library_items i
		WHERE i.kind IN ('movie', 'series')`))
	if err != nil {
		return fmt.Errorf("clear stale library frames: %w", err)
	}
	for rows.Next() {
		var id string
		var source sql.NullString
		if err := rows.Scan(&id, &source); err != nil {
			_ = rows.Close()
			return fmt.Errorf("clear stale library frames: %w", err)
		}
		if source.Valid {
			refs[id] = source.String
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("clear stale library frames: %w", err)
	}
	if len(refs) == 0 {
		return nil
	}
	waiting, err := s.mainFiles(ctx, s.h, waitingTitle(), ` AND EXISTS (SELECT 1 FROM library_item_images r
		WHERE r.item_id = i.id AND r.role = '`+LibraryFrameRole+`')`)
	if err != nil {
		return fmt.Errorf("clear stale library frames: %w", err)
	}
	for _, f := range waiting {
		if refs[f.ItemID] == f.file() {
			delete(refs, f.ItemID)
		}
	}
	for id := range refs {
		// It reads the main file again under the title's lock.
		if err := s.clearFrames(ctx, id, waitingTitle(), LibraryFrameRole); err != nil {
			return fmt.Errorf("clear library need frame %s: %w", id, err)
		}
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
