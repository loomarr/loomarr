package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LibraryAnswer is the maintainer's answer for a title nothing grounds (#2151 step 11): the TMDB
// title it is (an explicit link), or that it is not a title at all.
type LibraryAnswer struct {
	Origin LibraryOrigin
	// Kind is movie or series.
	Kind string
	// TMDBID is the linked title; "" when Ignored.
	TMDBID     string
	Ignored    bool
	AnsweredAt time.Time
}

// LibraryCandidate is one title a search found for a title nothing grounds, with whether its
// name, year and length agree with what is on disk.
type LibraryCandidate struct {
	Kind     string
	TMDBID   string
	Name     string
	Year     int
	Overview string
	// Poster is the catalog's poster URL; "" for none.
	Poster      string
	NameMatch   bool
	YearMatch   bool
	LengthMatch bool
}

// LibraryNeed is a title that needs the maintainer: present, nothing grounds it, and not answered.
type LibraryNeed struct {
	LibraryItemSummary
	Origin LibraryOrigin
	// Searched is what was last searched for it; "" never.
	Searched string
}

// LibraryNeedsStore keeps Needs you (#2151 step 11).
type LibraryNeedsStore interface {
	// LibraryNeeds lists every title that needs the maintainer, by sort name.
	LibraryNeeds(ctx context.Context) ([]LibraryNeed, error)
	// LibraryCandidates is each listed title's candidates, in rank order.
	LibraryCandidates(ctx context.Context, itemIDs []string) (map[string][]LibraryCandidate, error)
	// SaveLibraryCandidates replaces a title's candidates and notes what was searched.
	SaveLibraryCandidates(ctx context.Context, itemID, searched string, candidates []LibraryCandidate) error
	// LibraryItemOrigin is a title's kind and scan origin; ErrNotFound for an unknown title.
	LibraryItemOrigin(ctx context.Context, itemID string) (string, LibraryOrigin, error)
	// LibraryAnswerFor is the answer for an origin, and whether there is one.
	LibraryAnswerFor(ctx context.Context, origin LibraryOrigin) (LibraryAnswer, bool, error)
	// SaveLibraryAnswer records an answer, replacing any before it.
	SaveLibraryAnswer(ctx context.Context, answer LibraryAnswer) error
	// DeleteLibraryAnswer takes an answer back.
	DeleteLibraryAnswer(ctx context.Context, origin LibraryOrigin) error
	// RewriteLibraryItemFiles forgets the identity digest of a title's present files (a series'
	// episodes too), so the scanner writes them again, and lists them.
	RewriteLibraryItemFiles(ctx context.Context, itemID string) ([]LibraryFilePath, error)
	// UnnoticedLibraryNeeds lists the titles that need the maintainer and no notice has counted.
	UnnoticedLibraryNeeds(ctx context.Context) ([]string, error)
	// NoticeLibraryNeeds marks titles a notice counted.
	NoticeLibraryNeeds(ctx context.Context, itemIDs []string) error
}

func (s *sqlStore) UnnoticedLibraryNeeds(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT DISTINCT i.id FROM library_items i JOIN `+presentFiles+`
		f ON f.id = i.id WHERE `+needWhere+` AND i.needs_noticed = 0 ORDER BY i.id`))
	if err != nil {
		return nil, fmt.Errorf("unnoticed library needs: %w", err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("unnoticed library needs: %w", err)
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("unnoticed library needs: %w", err)
	}
	return ids, nil
}

func (s *sqlStore) NoticeLibraryNeeds(ctx context.Context, itemIDs []string) error {
	for _, id := range itemIDs {
		if _, err := s.db.ExecContext(ctx, s.ph(`UPDATE library_items SET needs_noticed = 1 WHERE id = ?`), id); err != nil {
			return fmt.Errorf("notice library needs: %w", err)
		}
	}
	return nil
}

// needWhere is a title that needs the maintainer: a movie or series nothing grounds, with a scan
// origin, and no answer for it.
const needWhere = `i.kind IN ('movie', 'series') AND i.grounded = 0 AND i.origin_key <> ''
	AND NOT EXISTS (SELECT 1 FROM library_answers a WHERE a.origin_authority = i.origin_authority
		AND a.origin_key = i.origin_key)`

func (s *sqlStore) LibraryNeeds(ctx context.Context) ([]LibraryNeed, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+itemColumns+`, i.origin_authority, i.origin_key, i.searched
		FROM library_items i JOIN `+presentFiles+` f ON f.id = i.id WHERE `+needWhere+`
		GROUP BY `+itemGroup+`, i.origin_authority, i.origin_key, i.searched ORDER BY i.sort_name, i.id`))
	if err != nil {
		return nil, fmt.Errorf("library needs: %w", err)
	}
	out := []LibraryNeed{}
	for rows.Next() {
		var n LibraryNeed
		var catalog string
		if err := rows.Scan(&n.ID, &n.Kind, &n.Name, &n.SortName, &n.Year, &catalog, &n.Match.Basis, &n.Match.App,
			&n.Match.Page, &n.Poster, &n.Files, &n.Pending, &n.Unplayable, &n.Origin.Authority, &n.Origin.Key, &n.Searched); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library needs: %w", err)
		}
		n.Catalog, _ = ParseLibraryCatalogKey(catalog)
		out = append(out, n)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library needs: %w", err)
	}
	return out, nil
}

func (s *sqlStore) LibraryCandidates(ctx context.Context, itemIDs []string) (map[string][]LibraryCandidate, error) {
	out := map[string][]LibraryCandidate{}
	for _, id := range itemIDs {
		rows, err := s.db.QueryContext(ctx, s.ph(`SELECT kind, tmdb_id, name, year, overview, poster, name_match,
			year_match, length_match FROM library_candidates WHERE item_id = ? ORDER BY rank`), id)
		if err != nil {
			return nil, fmt.Errorf("library candidates: %w", err)
		}
		for rows.Next() {
			var c LibraryCandidate
			if err := rows.Scan(&c.Kind, &c.TMDBID, &c.Name, &c.Year, &c.Overview, &c.Poster, &c.NameMatch,
				&c.YearMatch, &c.LengthMatch); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("library candidates: %w", err)
			}
			out[id] = append(out[id], c)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, fmt.Errorf("library candidates: %w", err)
		}
	}
	return out, nil
}

func (s *sqlStore) SaveLibraryCandidates(ctx context.Context, itemID, searched string, candidates []LibraryCandidate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save library candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_candidates WHERE item_id = ?`), itemID); err != nil {
		return fmt.Errorf("save library candidates: %w", err)
	}
	for rank, c := range candidates {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_candidates (item_id, rank, kind, tmdb_id, name,
			year, overview, poster, name_match, year_match, length_match) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			itemID, rank, c.Kind, c.TMDBID, c.Name, c.Year, c.Overview, c.Poster, c.NameMatch, c.YearMatch,
			c.LengthMatch); err != nil {
			return fmt.Errorf("save library candidate: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_items SET searched = ? WHERE id = ?`), searched, itemID); err != nil {
		return fmt.Errorf("save library candidates: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library candidates: %w", err)
	}
	return nil
}

func (s *sqlStore) LibraryItemOrigin(ctx context.Context, itemID string) (string, LibraryOrigin, error) {
	var kind string
	var o LibraryOrigin
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT kind, origin_authority, origin_key FROM library_items WHERE id = ?`),
		itemID).Scan(&kind, &o.Authority, &o.Key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", LibraryOrigin{}, ErrNotFound
	}
	if err != nil {
		return "", LibraryOrigin{}, fmt.Errorf("library item origin: %w", err)
	}
	return kind, o, nil
}

func (s *sqlStore) LibraryAnswerFor(ctx context.Context, origin LibraryOrigin) (LibraryAnswer, bool, error) {
	a := LibraryAnswer{Origin: origin}
	var at int64
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT kind, tmdb_id, ignored, answered_at FROM library_answers
		WHERE origin_authority = ? AND origin_key = ?`), origin.Authority, origin.Key).Scan(&a.Kind, &a.TMDBID, &a.Ignored, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return LibraryAnswer{}, false, nil
	}
	if err != nil {
		return LibraryAnswer{}, false, fmt.Errorf("library answer: %w", err)
	}
	a.AnsweredAt = time.UnixMilli(at).UTC()
	return a, true, nil
}

func (s *sqlStore) SaveLibraryAnswer(ctx context.Context, a LibraryAnswer) error {
	if a.Origin.Authority == "" || a.Origin.Key == "" || a.Kind == "" || a.AnsweredAt.IsZero() {
		return fmt.Errorf("save library answer: origin, kind and time are required")
	}
	if a.Ignored == (a.TMDBID != "") {
		return fmt.Errorf("save library answer: an answer is either a link or ignored")
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO library_answers
		(origin_authority, origin_key, kind, tmdb_id, ignored, answered_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (origin_authority, origin_key) DO UPDATE SET kind = excluded.kind, tmdb_id = excluded.tmdb_id,
		ignored = excluded.ignored, answered_at = excluded.answered_at`),
		a.Origin.Authority, a.Origin.Key, a.Kind, a.TMDBID, a.Ignored, a.AnsweredAt.UnixMilli()); err != nil {
		return fmt.Errorf("save library answer: %w", err)
	}
	return nil
}

func (s *sqlStore) DeleteLibraryAnswer(ctx context.Context, origin LibraryOrigin) error {
	if _, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM library_answers WHERE origin_authority = ? AND origin_key = ?`),
		origin.Authority, origin.Key); err != nil {
		return fmt.Errorf("delete library answer: %w", err)
	}
	return nil
}

func (s *sqlStore) RewriteLibraryItemFiles(ctx context.Context, itemID string) ([]LibraryFilePath, error) {
	return s.rewriteFiles(ctx, `item_id = ? OR series_id = ?`, itemID, itemID)
}

// rewriteFiles forgets the identity digest of the present files matching where, and lists them.
func (s *sqlStore) rewriteFiles(ctx context.Context, where string, args ...any) ([]LibraryFilePath, error) {
	return s.resetFiles(ctx, `identity = ''`, where, args...)
}

// resetFiles applies set to the present files matching where, and lists them, so a pass over
// their folders takes them up again.
func (s *sqlStore) resetFiles(ctx context.Context, set, where string, args ...any) ([]LibraryFilePath, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, s.ph(`SELECT root, path FROM library_files
		WHERE missing_at = 0 AND (`+where+`) ORDER BY path`), args...)
	if err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	var out []LibraryFilePath
	for rows.Next() {
		var f LibraryFilePath
		if err := rows.Scan(&f.Root, &f.Path); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("rewrite library files: %w", err)
		}
		out = append(out, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_files SET `+set+`
		WHERE missing_at = 0 AND (`+where+`)`), args...); err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	return out, nil
}
