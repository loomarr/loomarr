package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LibraryOrderChoice is which order a show's episode files follow (#2151 decision 53).
type LibraryOrderChoice string

const (
	// OrderByCatalog numbers the episodes by the catalog's own seasons (TMDB's "as first aired").
	OrderByCatalog LibraryOrderChoice = "catalog"
	// OrderByOrder numbers them by one of the catalog's other orders, named by OrderID.
	OrderByOrder LibraryOrderChoice = "order"
	// OrderByFiles keeps the name and number in each file.
	OrderByFiles LibraryOrderChoice = "files"
)

// LibraryEpisodeOrder is the household's answer for a show whose files the scanner could not
// number by its catalog: which order they follow.
type LibraryEpisodeOrder struct {
	// Series is the show's catalog row.
	Series LibraryCatalogKey
	Choice LibraryOrderChoice
	// OrderID is the catalog's id for the order; set only with OrderByOrder.
	OrderID    string
	AnsweredAt time.Time
}

// LibraryOrderNeed is a show with episodes the scanner could not number by its catalog, and
// nobody has said which order they follow.
type LibraryOrderNeed struct {
	LibraryItemSummary
	// Unnumbered counts its episodes that keep their file's name and number.
	Unnumbered int
	// Last is the highest episode on disk in each season, over all its present episodes: what an
	// order's season sizes are compared with.
	Last map[int]int
	// Waiting are the present files of its unnumbered episodes, in path order.
	Waiting []LibraryWaitingFile
}

// LibraryWaitingFile is a present file of an episode that keeps its file's name and number.
type LibraryWaitingFile struct {
	Path            string
	Season, Episode int
}

// LibraryEpisodeOrderStore keeps which order each show's episode files follow.
type LibraryEpisodeOrderStore interface {
	// LibraryEpisodeOrderFor is the answer for a show's catalog row, and whether there is one.
	LibraryEpisodeOrderFor(ctx context.Context, series LibraryCatalogKey) (LibraryEpisodeOrder, bool, error)
	// SaveLibraryEpisodeOrder records an answer, replacing any before it.
	SaveLibraryEpisodeOrder(ctx context.Context, answer LibraryEpisodeOrder) error
	// LibraryOrderNeeds lists the shows with an unnumbered present episode, by sort name.
	LibraryOrderNeeds(ctx context.Context) ([]LibraryOrderNeed, error)
}

// ErrBadEpisodeOrder is an answer whose choice and order id disagree.
var ErrBadEpisodeOrder = errors.New("an episode order names an order only when it picks one")

func (s *sqlStore) LibraryEpisodeOrderFor(ctx context.Context, series LibraryCatalogKey) (LibraryEpisodeOrder, bool, error) {
	out := LibraryEpisodeOrder{Series: series}
	var at int64
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT choice, order_id, answered_at_ms FROM library_episode_orders
		WHERE series_catalog = ?`), series.String()).Scan(&out.Choice, &out.OrderID, &at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return LibraryEpisodeOrder{}, false, nil
	case err != nil:
		return LibraryEpisodeOrder{}, false, fmt.Errorf("library episode order: %w", err)
	}
	out.AnsweredAt = time.UnixMilli(at)
	return out, true, nil
}

func (s *sqlStore) SaveLibraryEpisodeOrder(ctx context.Context, a LibraryEpisodeOrder) error {
	if a.Series.String() == "" {
		return errors.New("save library episode order: a show's catalog row is required")
	}
	switch a.Choice {
	case OrderByOrder:
		if a.OrderID == "" {
			return ErrBadEpisodeOrder
		}
	case OrderByCatalog, OrderByFiles:
		if a.OrderID != "" {
			return ErrBadEpisodeOrder
		}
	default:
		return fmt.Errorf("save library episode order: unknown choice %q", a.Choice)
	}
	if a.AnsweredAt.IsZero() {
		a.AnsweredAt = time.Now()
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO library_episode_orders
		(series_catalog, choice, order_id, answered_at_ms) VALUES (?, ?, ?, ?)
		ON CONFLICT (series_catalog) DO UPDATE SET choice = excluded.choice, order_id = excluded.order_id,
		answered_at_ms = excluded.answered_at_ms`),
		a.Series.String(), string(a.Choice), a.OrderID, a.AnsweredAt.UnixMilli()); err != nil {
		return fmt.Errorf("save library episode order: %w", err)
	}
	return nil
}

// presentEpisode is a series i's episode e with a present file.
const presentEpisode = `e.series_id = i.id AND e.kind = 'episode'
	AND EXISTS (SELECT 1 FROM library_files f WHERE f.inventory_item_id = e.id AND ` + presentFile + `)`

func (s *sqlStore) LibraryOrderNeeds(ctx context.Context) ([]LibraryOrderNeed, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+itemColumns+`,
		(SELECT COUNT(*) FROM library_items e WHERE `+presentEpisode+` AND e.unnumbered)
		FROM library_items i WHERE i.kind = 'series' AND `+itemListed+`
		AND EXISTS (SELECT 1 FROM library_items e WHERE `+presentEpisode+` AND e.unnumbered)
		ORDER BY i.sort_name, i.id`))
	if err != nil {
		return nil, fmt.Errorf("library order needs: %w", err)
	}
	out := []LibraryOrderNeed{}
	for rows.Next() {
		var n LibraryOrderNeed
		var catalog, set string
		if err := rows.Scan(&n.ID, &n.Kind, &n.Name, &n.SortName, &n.Year, &catalog, &n.Match.Basis, &n.Match.App,
			&n.Match.Page, &set, &n.Poster, &n.Backdrop, &n.Logo, &n.Files, &n.Pending, &n.Unplayable, &n.ExtraBytes,
			&n.Unnumbered); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library order needs: %w", err)
		}
		n.Catalog, _ = ParseLibraryCatalogKey(catalog)
		n.Set, _ = ParseLibraryCatalogKey(set)
		out = append(out, n)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library order needs: %w", err)
	}
	for i := range out {
		last, err := s.lastEpisodes(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		waiting, err := s.waitingFiles(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Last, out[i].Waiting = last, waiting
	}
	return out, nil
}

// waitingFiles is the present files of a series' unnumbered episodes, in path order.
func (s *sqlStore) waitingFiles(ctx context.Context, seriesID string) ([]LibraryWaitingFile, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT f.path, e.season, e.episode FROM library_items e
		JOIN library_files f ON f.inventory_item_id = e.id AND `+presentFile+`
		WHERE e.series_id = ? AND e.kind = 'episode' AND e.unnumbered ORDER BY f.path`), seriesID)
	if err != nil {
		return nil, fmt.Errorf("library waiting files: %w", err)
	}
	out := []LibraryWaitingFile{}
	for rows.Next() {
		var f LibraryWaitingFile
		if err := rows.Scan(&f.Path, &f.Season, &f.Episode); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library waiting files: %w", err)
		}
		out = append(out, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library waiting files: %w", err)
	}
	return out, nil
}

// lastEpisodes is the highest episode on disk in each of a series' seasons, a multi-episode file
// counted through its last episode.
func (s *sqlStore) lastEpisodes(ctx context.Context, seriesID string) (map[int]int, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT e.season, MAX(e.episode), MAX(e.episode_end)
		FROM library_items i JOIN library_items e ON `+presentEpisode+`
		WHERE i.id = ? AND e.episode > 0 GROUP BY e.season`), seriesID)
	if err != nil {
		return nil, fmt.Errorf("library last episodes: %w", err)
	}
	out := map[int]int{}
	for rows.Next() {
		var season, episode, end int
		if err := rows.Scan(&season, &episode, &end); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library last episodes: %w", err)
		}
		out[season] = max(episode, end)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library last episodes: %w", err)
	}
	return out, nil
}
