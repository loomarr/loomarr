package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// LibraryAnswer is the answer for a title nothing grounds (#2151 step 11): the catalog title it is
// (a link), that it is not a title at all, or that it is the household's own, kept as it is.
type LibraryAnswer struct {
	Origin LibraryOrigin
	// Kind is movie or series.
	Kind string
	// ID is the linked title, by any catalog's id; zero when Ignored or Kept.
	ID      LibraryID
	Ignored bool
	// Kept is a home video.
	Kept       bool
	AnsweredAt time.Time
	// By is who answered; "" saves as the maintainer. Only the maintainer ignores or keeps a title.
	By LibraryAnswerBy
}

// LibraryAnswerBy is who answered a title.
type LibraryAnswerBy string

const (
	// AnsweredByYou is the maintainer, in Needs you or "Wrong title?".
	AnsweredByYou LibraryAnswerBy = "you"
	// AnsweredBySearch is the Matcher: exactly one catalog result agreed on the title's name, year
	// and length (decision 56). A maintainer's answer replaces it.
	AnsweredBySearch LibraryAnswerBy = "search"
)

// LibraryCandidate is one title a search found for a title nothing grounds, with whether its
// name, year and length agree with what is on disk.
type LibraryCandidate struct {
	Kind string
	// ID is the title by the id of the catalog that found it.
	ID       LibraryID
	Name     string
	Year     int
	Overview string
	// Poster is the catalog's poster URL; "" for none.
	Poster    string
	NameMatch bool
	YearMatch bool
	// LengthMatch is whether what the files say beyond the name agrees: a movie's length, or a
	// series' seasons and episodes (decisions 56 and 57).
	LengthMatch bool
}

// LibraryNeed is a title that needs the maintainer: present, nothing grounds it, and not answered.
type LibraryNeed struct {
	LibraryItemSummary
	Origin LibraryOrigin
	// Searched is what was last searched for it; "" never.
	Searched string
}

// LibraryIgnoredTitle is a title the maintainer said is not one.
type LibraryIgnoredTitle struct {
	ID string
	// Kind is movie or series.
	Kind      string
	Name      string
	Origin    LibraryOrigin
	IgnoredAt time.Time
}

// LibraryAnswered is a title a batch answer or restore touched.
type LibraryAnswered struct {
	ID     string
	Origin LibraryOrigin
}

var (
	// ErrNotNeeded is a batch ignore naming a title that is not waiting in Needs you.
	ErrNotNeeded = errors.New("library: that title does not need you")
	// ErrNotIgnored is a batch restore naming a title that is not ignored.
	ErrNotIgnored = errors.New("library: that title is not ignored")
	// ErrNotKeepable is a keep of a title that is not waiting in Needs you, or that has a guess.
	ErrNotKeepable = errors.New("library: only a title that needs you and has no guess can be kept")
)

// LibraryNeedsStore keeps Needs you (#2151 step 11).
type LibraryNeedsStore interface {
	// IgnoreLibraryNeeds ignores titles that need the maintainer, all in one transaction:
	// ErrNotNeeded, and nothing saved, if any is not waiting in Needs you.
	IgnoreLibraryNeeds(ctx context.Context, itemIDs []string, at time.Time) ([]LibraryAnswered, error)
	// KeepLibraryNeed answers a title that needs the maintainer and has no candidate as a home
	// video: ErrNotKeepable, and nothing saved, otherwise.
	KeepLibraryNeed(ctx context.Context, itemID string, at time.Time) (LibraryAnswered, error)
	// HomeVideosWithoutPoster returns the kept titles that have no poster, each with its main file.
	HomeVideosWithoutPoster(ctx context.Context) ([]LibraryHomeVideoFrame, error)
	// KeptLibraryTitle reports whether a file belongs to a title kept as a home video.
	KeptLibraryTitle(ctx context.Context, kind string, origin LibraryOrigin, groups []LibraryID) (bool, error)
	// RestoreLibraryIgnored takes back the ignore of titles, all in one transaction:
	// ErrNotIgnored, and nothing removed, if any is not ignored.
	RestoreLibraryIgnored(ctx context.Context, itemIDs []string) ([]LibraryAnswered, error)
	// LibraryIgnored is a page of the ignored titles with a present file, newest ignore first, and
	// how many there are in all.
	LibraryIgnored(ctx context.Context, limit, offset int) ([]LibraryIgnoredTitle, int, error)
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

func (s *Store) LibraryIgnored(ctx context.Context, limit, offset int) ([]LibraryIgnoredTitle, int, error) {
	from := ` FROM library_items i JOIN library_answers a ON a.origin_authority = i.origin_authority
		AND a.origin_key = i.origin_key
		WHERE i.kind IN ('movie', 'series') AND a.ignored = TRUE AND ` + itemListed
	var total int
	if err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*)`+from)).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("library ignored: %w", err)
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT i.id, i.kind, i.name, i.origin_authority, i.origin_key,
		a.answered_at_ms`+from+` ORDER BY a.answered_at_ms DESC, i.id LIMIT ? OFFSET ?`), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("library ignored: %w", err)
	}
	out := []LibraryIgnoredTitle{}
	for rows.Next() {
		var t LibraryIgnoredTitle
		var at int64
		if err := rows.Scan(&t.ID, &t.Kind, &t.Name, &t.Origin.Authority, &t.Origin.Key, &at); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("library ignored: %w", err)
		}
		t.IgnoredAt = time.UnixMilli(at).UTC()
		out = append(out, t)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, 0, fmt.Errorf("library ignored: %w", err)
	}
	return out, total, nil
}

func (s *Store) UnnoticedLibraryNeeds(ctx context.Context) ([]string, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT i.id FROM library_items i
		WHERE `+needWhere+` AND `+itemListed+` AND i.needs_noticed = FALSE ORDER BY i.id`))
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

func (s *Store) NoticeLibraryNeeds(ctx context.Context, itemIDs []string) error {
	for _, id := range itemIDs {
		if _, err := s.h.ExecContext(ctx, s.ph(`UPDATE library_items SET needs_noticed = TRUE WHERE id = ?`), id); err != nil {
			return fmt.Errorf("notice library needs: %w", err)
		}
	}
	return nil
}

// needWhere is a title that needs the maintainer: a movie or series nothing grounds, with a scan
// origin, and no answer for it.
const needWhere = `i.kind IN ('movie', 'series') AND i.grounded = FALSE AND i.origin_key <> ''
	AND NOT EXISTS (SELECT 1 FROM library_answers a WHERE a.origin_authority = i.origin_authority
		AND a.origin_key = i.origin_key)`

func (s *Store) LibraryNeeds(ctx context.Context) ([]LibraryNeed, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT `+itemColumns+`, i.origin_authority, i.origin_key, i.searched
		FROM library_items i WHERE `+needWhere+` AND NOT `+titleKept("i")+` AND `+itemListed+` ORDER BY i.sort_name, i.id`))
	if err != nil {
		return nil, fmt.Errorf("library needs: %w", err)
	}
	out := []LibraryNeed{}
	for rows.Next() {
		var n LibraryNeed
		var catalog, set string
		if err := rows.Scan(&n.ID, &n.Kind, &n.Name, &n.SortName, &n.Year, &catalog, &n.Match.Basis, &n.Match.App,
			&n.Match.Page, &set, &n.Poster, &n.Backdrop, &n.Logo, &n.Files, &n.Pending, &n.Unplayable, &n.ExtraCopies, &n.ExtraBytes, &n.HomeVideo, &n.Unidentified, &n.Origin.Authority, &n.Origin.Key,
			&n.Searched); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library needs: %w", err)
		}
		n.Catalog, _ = ParseLibraryCatalogKey(catalog)
		n.Set, _ = ParseLibraryCatalogKey(set)
		out = append(out, n)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library needs: %w", err)
	}
	return out, nil
}

func (s *Store) LibraryCandidates(ctx context.Context, itemIDs []string) (map[string][]LibraryCandidate, error) {
	out := map[string][]LibraryCandidate{}
	for len(itemIDs) > 0 {
		batch := itemIDs[:min(len(itemIDs), idsPerRead)]
		itemIDs = itemIDs[len(batch):]
		if err := s.readCandidates(ctx, batch, out); err != nil {
			return nil, fmt.Errorf("library candidates: %w", err)
		}
	}
	return out, nil
}

// readCandidates adds a batch of titles' candidates to out, each title's in rank order.
func (s *Store) readCandidates(ctx context.Context, ids []string, out map[string][]LibraryCandidate) error {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT inventory_item_id, kind, namespace, external_id, name, year, overview,
		poster, name_match, year_match, length_match FROM library_candidates
		WHERE inventory_item_id IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")+`)
		ORDER BY inventory_item_id, rank`), args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var c LibraryCandidate
		if err := rows.Scan(&id, &c.Kind, &c.ID.Namespace, &c.ID.Value, &c.Name, &c.Year, &c.Overview, &c.Poster,
			&c.NameMatch, &c.YearMatch, &c.LengthMatch); err != nil {
			_ = rows.Close()
			return err
		}
		out[id] = append(out[id], c)
	}
	return errors.Join(rows.Err(), rows.Close())
}

func (s *Store) SaveLibraryCandidates(ctx context.Context, itemID, searched string, candidates []LibraryCandidate) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("save library candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_candidates WHERE inventory_item_id = ?`), itemID); err != nil {
		return fmt.Errorf("save library candidates: %w", err)
	}
	for rank, c := range candidates {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_candidates (inventory_item_id, rank, kind, namespace,
			external_id, name, year, overview, poster, name_match, year_match, length_match)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			itemID, rank, c.Kind, c.ID.Namespace, c.ID.Value, c.Name, c.Year, c.Overview, c.Poster, c.NameMatch, c.YearMatch,
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

func (s *Store) LibraryItemOrigin(ctx context.Context, itemID string) (string, LibraryOrigin, error) {
	var kind string
	var o LibraryOrigin
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT kind, origin_authority, origin_key FROM library_items WHERE id = ?`),
		itemID).Scan(&kind, &o.Authority, &o.Key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", LibraryOrigin{}, storedb.ErrNotFound
	}
	if err != nil {
		return "", LibraryOrigin{}, fmt.Errorf("library item origin: %w", err)
	}
	return kind, o, nil
}

func (s *Store) LibraryAnswerFor(ctx context.Context, origin LibraryOrigin) (LibraryAnswer, bool, error) {
	a := LibraryAnswer{Origin: origin}
	var at int64
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT kind, namespace, external_id, ignored, kept, answered_at_ms, answered_by
		FROM library_answers WHERE origin_authority = ? AND origin_key = ?`), origin.Authority, origin.Key).Scan(
		&a.Kind, &a.ID.Namespace, &a.ID.Value, &a.Ignored, &a.Kept, &at, &a.By)
	if errors.Is(err, sql.ErrNoRows) {
		return LibraryAnswer{}, false, nil
	}
	if err != nil {
		return LibraryAnswer{}, false, fmt.Errorf("library answer: %w", err)
	}
	a.AnsweredAt = time.UnixMilli(at).UTC()
	return a, true, nil
}

func (s *Store) SaveLibraryAnswer(ctx context.Context, a LibraryAnswer) error {
	if a.Origin.Authority == "" || a.Origin.Key == "" || a.Kind == "" || a.AnsweredAt.IsZero() {
		return fmt.Errorf("save library answer: origin, kind and time are required")
	}
	linked := a.ID != LibraryID{}
	if a.Ignored && a.Kept || linked == (a.Ignored || a.Kept) {
		return fmt.Errorf("save library answer: an answer is exactly one of a link, ignored or kept")
	}
	if linked && (a.ID.Namespace == "" || a.ID.Value == "") {
		return fmt.Errorf("save library answer: a link names a catalog and an id")
	}
	if a.By == "" {
		a.By = AnsweredByYou
	}
	if a.By != AnsweredByYou && (a.By != AnsweredBySearch || !linked) {
		return fmt.Errorf("save library answer: %q cannot give this answer", a.By)
	}
	// A search never replaces the maintainer's answer (decision 58), even one given between the
	// Matcher's look and this write; a home video is one, on whichever origin of the title.
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("save library answer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if a.By == AnsweredByYou {
		if err := s.clearTitleKept(ctx, tx, a.Origin); err != nil {
			return fmt.Errorf("save library answer: %w", err)
		}
	} else {
		query, args := keptQuery(a.Kind, a.Origin, nil)
		var kept bool
		if err := tx.QueryRowContext(ctx, s.ph(query), args...).Scan(&kept); err != nil {
			return fmt.Errorf("save library answer: %w", err)
		}
		if kept {
			return nil
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_answers
		(origin_authority, origin_key, kind, namespace, external_id, ignored, kept, answered_at_ms, answered_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (origin_authority, origin_key) DO UPDATE SET kind = excluded.kind, namespace = excluded.namespace,
		external_id = excluded.external_id, ignored = excluded.ignored, kept = excluded.kept,
		answered_at_ms = excluded.answered_at_ms, answered_by = excluded.answered_by
		WHERE excluded.answered_by = ? OR library_answers.answered_by <> ?`),
		a.Origin.Authority, a.Origin.Key, a.Kind, a.ID.Namespace, a.ID.Value, a.Ignored, a.Kept, a.AnsweredAt.UnixMilli(),
		string(a.By), string(AnsweredByYou), string(AnsweredByYou)); err != nil {
		return fmt.Errorf("save library answer: %w", err)
	}
	if a.Kept {
		if _, err := s.keepTitle(ctx, tx, a.Origin, a.Kind, a.AnsweredAt.UnixMilli()); err != nil {
			return fmt.Errorf("save library answer: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library answer: %w", err)
	}
	return nil
}

func (s *Store) DeleteLibraryAnswer(ctx context.Context, origin LibraryOrigin) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete library answer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.clearTitleKept(ctx, tx, origin); err != nil {
		return fmt.Errorf("delete library answer: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM library_answers WHERE origin_authority = ? AND origin_key = ?`),
		origin.Authority, origin.Key); err != nil {
		return fmt.Errorf("delete library answer: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete library answer: %w", err)
	}
	return nil
}

func (s *Store) RewriteLibraryItemFiles(ctx context.Context, itemID string) ([]LibraryFilePath, error) {
	return s.rewriteFiles(ctx, `inventory_item_id = ? OR inventory_series_id = ?`, itemID, itemID)
}

// rewriteFiles forgets the identity digest of the present files matching where, and lists them.
func (s *Store) rewriteFiles(ctx context.Context, where string, args ...any) ([]LibraryFilePath, error) {
	return s.resetFiles(ctx, `identity = ''`, where, args...)
}

// resetFiles applies set to the present files matching where, and lists them, so a pass over
// their folders takes them up again.
func (s *Store) resetFiles(ctx context.Context, set, where string, args ...any) ([]LibraryFilePath, error) {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, s.ph(`SELECT root, path FROM library_files
		WHERE missing_at_ms = 0 AND (`+where+`) ORDER BY path`), args...)
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
		WHERE missing_at_ms = 0 AND (`+where+`)`), args...); err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("rewrite library files: %w", err)
	}
	return out, nil
}
