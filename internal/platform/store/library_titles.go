package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LibraryTitle is a movie or series the Library holds with a file to play (#2151 step 15), by the
// provider ids a title record knows it by: what availability, presence and ratings asked the media
// server until the cutover.
type LibraryTitle struct {
	ID string
	// Kind is movie or series.
	Kind   string
	Name   string
	TMDBID string
	TVDBID string
	Rating string
	Genres []string
	// SizeBytes is a movie's playing file's size; 0 for a series.
	SizeBytes int64
}

// LibraryTitleReader reads the Library's titles that have a file to play.
type LibraryTitleReader interface {
	// LibraryTitles lists the movies and series with a present file whose record changed at or
	// after since; the zero time lists them all.
	LibraryTitles(ctx context.Context, since time.Time) ([]LibraryTitle, error)
	// LibraryTitleByProviderID is the movie or series (kind) with a present file that the provider
	// id names (namespace tmdb or tvdb). ok is false when the Library holds none.
	LibraryTitleByProviderID(ctx context.Context, kind, namespace, value string) (title LibraryTitle, ok bool, err error)
}

const libraryTitleColumns = `i.id, i.kind, i.name,
	COALESCE((SELECT MIN(d.value) FROM library_ids d WHERE d.item_id = i.id AND d.namespace = 'tmdb'), ''),
	COALESCE((SELECT MIN(d.value) FROM library_ids d WHERE d.item_id = i.id AND d.namespace = 'tvdb'), ''),
	i.rating,
	CASE WHEN i.kind = 'movie' THEN COALESCE((SELECT MAX(s.size_bytes) FROM library_sources s
		WHERE s.item_id = i.id AND s.plays), 0) ELSE 0 END
	FROM library_items i`

func (s *sqlStore) LibraryTitles(ctx context.Context, since time.Time) ([]LibraryTitle, error) {
	var sinceMs int64
	if !since.IsZero() {
		sinceMs = since.UnixMilli()
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+libraryTitleColumns+`
		WHERE i.kind IN ('movie', 'series') AND i.updated_at_ms >= ? AND `+itemListed+`
		ORDER BY i.id`), sinceMs)
	if err != nil {
		return nil, fmt.Errorf("library titles: %w", err)
	}
	var out []LibraryTitle
	for rows.Next() {
		var t LibraryTitle
		if err := rows.Scan(&t.ID, &t.Kind, &t.Name, &t.TMDBID, &t.TVDBID, &t.Rating, &t.SizeBytes); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library titles: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("library titles: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("library titles: %w", err)
	}
	return out, nil
}

func (s *sqlStore) LibraryTitleByProviderID(ctx context.Context, kind, namespace, value string) (LibraryTitle, bool, error) {
	var t LibraryTitle
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT `+libraryTitleColumns+`
		WHERE i.kind = ? AND i.id IN (SELECT item_id FROM library_ids WHERE namespace = ? AND value = ?)
		AND `+itemListed+` ORDER BY i.id LIMIT 1`), kind, namespace, value).Scan(
		&t.ID, &t.Kind, &t.Name, &t.TMDBID, &t.TVDBID, &t.Rating, &t.SizeBytes)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return LibraryTitle{}, false, nil
	case err != nil:
		return LibraryTitle{}, false, fmt.Errorf("library title by %s id: %w", namespace, err)
	}
	genres, err := s.libraryGenres(ctx, t.ID)
	if err != nil {
		return LibraryTitle{}, false, err
	}
	t.Genres = genres
	return t, true, nil
}

// LibraryTitleSearcher finds the Library's titles by name: the library half of title search
// (#2151 step 15e), what the media server's search answered until the cutover.
type LibraryTitleSearcher interface {
	// SearchLibraryTitles is up to limit movies and series with a present file whose name holds
	// term (case-insensitive, LIKE's wildcards taken literally): an exact name first, then one
	// starting with it, then the rest, each by sort name. Genres are not filled.
	SearchLibraryTitles(ctx context.Context, term string, limit int) ([]LibraryTitle, error)
}

func (s *sqlStore) SearchLibraryTitles(ctx context.Context, term string, limit int) ([]LibraryTitle, error) {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" || limit <= 0 {
		return nil, nil
	}
	escaped := likeEscape(term)
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+libraryTitleColumns+`
		WHERE i.kind IN ('movie', 'series') AND LOWER(i.name) LIKE ? ESCAPE '\' AND `+itemListed+`
		ORDER BY CASE WHEN LOWER(i.name) = ? THEN 0 WHEN LOWER(i.name) LIKE ? ESCAPE '\' THEN 1 ELSE 2 END,
			i.sort_name, i.id
		LIMIT ?`), "%"+escaped+"%", term, escaped+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search library titles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LibraryTitle
	for rows.Next() {
		var t LibraryTitle
		if err := rows.Scan(&t.ID, &t.Kind, &t.Name, &t.TMDBID, &t.TVDBID, &t.Rating, &t.SizeBytes); err != nil {
			return nil, fmt.Errorf("search library titles: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search library titles: %w", err)
	}
	return out, nil
}

// LibraryProviderID names a movie or series (Kind) by one provider id (Namespace tmdb or tvdb):
// how an airing's provisioning key names the title it plays.
type LibraryProviderID struct {
	Kind      string
	Namespace string
	Value     string
}

// LibraryTitleArt is a title's backdrop and clear logo, as image hashes; "" for one it lacks.
type LibraryTitleArt struct {
	Backdrop string
	Logo     string
}

// LibraryArtReader reads the art of the titles a guide's airings play (#2567).
type LibraryArtReader interface {
	// LibraryTitleArt is the backdrop and logo of each named movie or series the Library holds
	// with a file to play. An id the Library doesn't hold, or whose title has neither image, is
	// absent from the map.
	LibraryTitleArt(ctx context.Context, ids []LibraryProviderID) (map[LibraryProviderID]LibraryTitleArt, error)
}

func (s *sqlStore) LibraryTitleArt(ctx context.Context, ids []LibraryProviderID) (map[LibraryProviderID]LibraryTitleArt, error) {
	out := map[LibraryProviderID]LibraryTitleArt{}
	if len(ids) == 0 {
		return out, nil
	}
	match := make([]string, 0, len(ids))
	args := make([]any, 0, 3*len(ids))
	for _, id := range ids {
		match = append(match, `(i.kind = ? AND d.namespace = ? AND d.value = ?)`)
		args = append(args, id.Kind, id.Namespace, id.Value)
	}
	// Ordered by item id so two titles sharing a provider id resolve as LibraryTitleByProviderID
	// does: the first one wins.
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT i.kind, d.namespace, d.value,
		`+itemImage("backdrop")+`, `+itemImage("logo")+`
		FROM library_items i JOIN library_ids d ON d.item_id = i.id
		WHERE (`+strings.Join(match, ` OR `)+`) AND `+itemListed+`
		ORDER BY i.id`), args...)
	if err != nil {
		return nil, fmt.Errorf("library title art: %w", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[LibraryProviderID]bool{}
	for rows.Next() {
		var id LibraryProviderID
		var art LibraryTitleArt
		if err := rows.Scan(&id.Kind, &id.Namespace, &id.Value, &art.Backdrop, &art.Logo); err != nil {
			return nil, fmt.Errorf("library title art: %w", err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if art != (LibraryTitleArt{}) {
			out[id] = art
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("library title art: %w", err)
	}
	return out, nil
}

func (s *sqlStore) libraryGenres(ctx context.Context, itemID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT name FROM library_genres WHERE item_id = ? ORDER BY position`), itemID)
	if err != nil {
		return nil, fmt.Errorf("library genres: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("library genres: %w", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("library genres: %w", err)
	}
	return out, nil
}
