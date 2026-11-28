package librarystore

import (
	"context"
	"errors"
)

// LibraryHomeVideoFrame is where a home video's picture comes from: the main file of a kept
// movie or show that has no poster.
type LibraryHomeVideoFrame struct {
	ItemID     string
	Path       string
	DurationMs int64
	// HDR says the file's picture is PQ or HLG, so a frame of it is tone-mapped.
	HDR bool
}

// HomeVideosWithoutPoster returns each kept title that has no poster ref and has a file of known
// length, with its main file: a movie's first playing part, or a show's first playing episode
// file by path. In item id order.
func (s *Store) HomeVideosWithoutPoster(ctx context.Context) ([]LibraryHomeVideoFrame, error) {
	hdr := `EXISTS (SELECT 1 FROM library_streams v WHERE v.item_id = s.item_id AND v.path = s.path
		AND v.color_transfer IN ('smpte2084', 'arib-std-b67'))`
	wanted := titleKept("i") + ` AND NOT EXISTS (SELECT 1 FROM library_item_images r
		WHERE r.item_id = i.id AND r.role = 'poster') AND s.plays AND s.duration_ms > 0`
	rows, err := s.h.QueryContext(ctx, `SELECT i.id AS id, s.path AS path, s.duration_ms, `+hdr+`, s.part AS part
			FROM library_items i JOIN library_sources s ON s.item_id = i.id
			WHERE i.kind = 'movie' AND `+wanted+`
		UNION ALL SELECT i.id, s.path, s.duration_ms, `+hdr+`, s.part
			FROM library_items i JOIN library_files f ON f.inventory_series_id = i.id AND `+presentFile+`
			JOIN library_sources s ON s.item_id = f.inventory_item_id AND s.path = f.path
			WHERE i.kind = 'series' AND `+wanted+`
		ORDER BY id, path, part`)
	if err != nil {
		return nil, err
	}
	var out []LibraryHomeVideoFrame
	part := 0
	for rows.Next() {
		var f LibraryHomeVideoFrame
		var p int
		if err := rows.Scan(&f.ItemID, &f.Path, &f.DurationMs, &f.HDR, &p); err != nil {
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
