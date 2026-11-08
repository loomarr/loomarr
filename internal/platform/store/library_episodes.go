package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// LibraryEpisode is one playable episode of a library series, as a lineup expands it (#2151 step
// 15): the facts the media server's episode listing answered until the cutover.
type LibraryEpisode struct {
	ItemID     string
	Name       string
	Season     int
	Episode    int
	EpisodeEnd int
	Year       int
	// PremiereDate is the air date as the catalog gave it, YYYY-MM-DD or empty.
	PremiereDate string
	// Rating is the episode's own content rating, else its series' (copied when either is saved).
	Rating   string
	Score    float64
	Overview string
	Tags     []string
	// DurationMs is the playing file's own duration, else the catalog's runtime.
	DurationMs int64
}

// LibraryEpisodeReader lists a library series' episodes.
type LibraryEpisodeReader interface {
	// LibraryEpisodes returns the series' episodes that have a file to play, in season and episode
	// order. found is false when seriesID is no library series.
	LibraryEpisodes(ctx context.Context, seriesID string) (eps []LibraryEpisode, found bool, err error)
}

func (s *sqlStore) LibraryEpisodes(ctx context.Context, seriesID string) ([]LibraryEpisode, bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT 1 FROM library_items WHERE id = ? AND kind = 'series'`),
		seriesID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("library series %s: %w", seriesID, err)
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT i.id, i.name, i.season, i.episode, i.episode_end, i.year,
		i.premiere_date, i.rating, i.score, i.overview,
		COALESCE(NULLIF((SELECT MAX(src.duration_ms) FROM library_sources src WHERE src.item_id = i.id AND src.plays), 0), i.runtime_ms)
		FROM library_items i
		WHERE i.series_id = ? AND i.kind = 'episode' AND EXISTS (
			SELECT 1 FROM library_sources src JOIN library_files f ON f.path = src.path
			WHERE src.item_id = i.id AND src.plays AND f.missing_at_ms = 0 AND f.problem = '')
		ORDER BY i.season, i.episode, i.id`), seriesID)
	if err != nil {
		return nil, true, fmt.Errorf("library episodes of %s: %w", seriesID, err)
	}
	var eps []LibraryEpisode
	at := map[string]int{}
	for rows.Next() {
		var e LibraryEpisode
		if err := rows.Scan(&e.ItemID, &e.Name, &e.Season, &e.Episode, &e.EpisodeEnd, &e.Year,
			&e.PremiereDate, &e.Rating, &e.Score, &e.Overview, &e.DurationMs); err != nil {
			_ = rows.Close()
			return nil, true, fmt.Errorf("library episodes of %s: %w", seriesID, err)
		}
		at[e.ItemID] = len(eps)
		eps = append(eps, e)
	}
	if err := rows.Close(); err != nil {
		return nil, true, err
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("library episodes of %s: %w", seriesID, err)
	}
	trows, err := s.db.QueryContext(ctx, s.ph(`SELECT t.item_id, t.name FROM library_tags t
		JOIN library_items i ON i.id = t.item_id
		WHERE i.series_id = ? AND i.kind = 'episode' ORDER BY t.item_id, t.position`), seriesID)
	if err != nil {
		return nil, true, fmt.Errorf("library episode tags of %s: %w", seriesID, err)
	}
	defer func() { _ = trows.Close() }()
	for trows.Next() {
		var id, name string
		if err := trows.Scan(&id, &name); err != nil {
			return nil, true, fmt.Errorf("library episode tags of %s: %w", seriesID, err)
		}
		if i, ok := at[id]; ok {
			eps[i].Tags = append(eps[i].Tags, name)
		}
	}
	if err := trows.Err(); err != nil {
		return nil, true, fmt.Errorf("library episode tags of %s: %w", seriesID, err)
	}
	return eps, true, nil
}
