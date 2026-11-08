package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
