package librarystore

import (
	"context"
	"errors"
	"fmt"
)

// The copies of a title (#2151 decisions 37 and 54, #4476): which of its files plays, which is
// an extra copy, and which can't be played. One rule decides it, here, and every surface reads
// it: the title page's files, the counts on the wall and in Needs you, and the guard on removing
// an extra copy.

// LibraryCopyState is what one of a title's files is to it.
type LibraryCopyState string

const (
	// LibraryCopyPlays is a copy that plays when the title airs.
	LibraryCopyPlays LibraryCopyState = "plays"
	// LibraryCopyExtra is a copy that never plays, because another of the title's listed,
	// readable files plays in its place: the same edition, and the same part or a whole file.
	LibraryCopyExtra LibraryCopyState = "extra"
	// LibraryCopyUnplayable is a file whose header could not be read, with no copy that plays in
	// its place.
	LibraryCopyUnplayable LibraryCopyState = "unplayable"
	// LibraryCopyReading is a file whose header is not read yet, or that has no source row yet.
	LibraryCopyReading LibraryCopyState = "reading"
	// LibraryCopyWaiting is a copy that does not play while the copy that plays in its place is
	// not listed or can't be read: it is neither an extra nor broken. It stays so for as long as
	// that copy is the one that plays: a broken copy the arr manages keeps playing, since the walk
	// ranks the arr's file first, so the readable copy beside it waits.
	LibraryCopyWaiting LibraryCopyState = "waiting"
)

// LibraryCopy is one of a title's own present files and what it is to the title.
type LibraryCopy struct {
	Path  string
	State LibraryCopyState
	// InPlaceOf is the paths of the listed files that play in an extra copy's place, in path
	// order; nil for any other state.
	InPlaceOf []string
}

// playsInPlace joins, to a source row s, each other source row p of its title that plays in its
// place (the same edition, and the same part or a whole file), and p's file g, present,
// importable and readable for the same title.
const playsInPlace = `JOIN library_sources p ON p.item_id = s.item_id AND p.path <> s.path AND p.plays
		AND p.edition = s.edition AND (p.part = s.part OR p.part = 0)
	JOIN library_files g ON g.path = p.path AND g.inventory_item_id = p.item_id
		AND g.missing_at_ms = 0 AND g.problem = '' AND g.header <> 'failed'`

// replaceable is the condition on a file f and its source row s that f does not play in its own
// right: its row does not play, or its header could not be read.
const replaceable = `(NOT s.plays OR f.header = 'failed')`

// replacedFile is the condition on a library_files row f that it is an extra copy: a copy that
// plays in its place is listed and readable.
const replacedFile = `EXISTS (SELECT 1 FROM library_sources s ` + playsInPlace + `
		WHERE s.item_id = f.inventory_item_id AND s.path = f.path AND ` + replaceable + `)`

// LibraryItemCopies lists the item's own present files, in path order, with what each is to it.
// A series has none: its files are its episodes'.
func (s *Store) LibraryItemCopies(ctx context.Context, id string) ([]LibraryCopy, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT f.path, CASE
			WHEN `+replacedFile+` THEN 'extra'
			WHEN f.header = 'failed' THEN 'unplayable'
			WHEN f.header = 'pending' OR src.path IS NULL THEN 'reading'
			WHEN src.plays THEN 'plays'
			ELSE 'waiting' END
		FROM library_files f LEFT JOIN library_sources src ON src.item_id = f.inventory_item_id AND src.path = f.path
		WHERE f.inventory_item_id = ? AND `+presentFile+` ORDER BY f.path`), id)
	if err != nil {
		return nil, fmt.Errorf("library item copies: %w", err)
	}
	var out []LibraryCopy
	at := map[string]int{}
	for rows.Next() {
		var c LibraryCopy
		if err := rows.Scan(&c.Path, &c.State); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library item copies: %w", err)
		}
		at[c.Path] = len(out)
		out = append(out, c)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library item copies: %w", err)
	}
	rows, err = s.h.QueryContext(ctx, s.ph(`SELECT f.path, p.path FROM library_files f
		JOIN library_sources s ON s.item_id = f.inventory_item_id AND s.path = f.path `+playsInPlace+`
		WHERE f.inventory_item_id = ? AND `+presentFile+` AND `+replaceable+` ORDER BY f.path, p.path`), id)
	if err != nil {
		return nil, fmt.Errorf("library item copies: %w", err)
	}
	for rows.Next() {
		var path, twin string
		if err := rows.Scan(&path, &twin); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library item copies: %w", err)
		}
		if i, ok := at[path]; ok && out[i].State == LibraryCopyExtra {
			out[i].InPlaceOf = append(out[i].InPlaceOf, twin)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library item copies: %w", err)
	}
	return out, nil
}
