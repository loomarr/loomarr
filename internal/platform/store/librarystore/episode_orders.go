package librarystore

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
	// LibraryOrderNeedCount is how many shows LibraryOrderNeeds lists, without listing them: what
	// the Needs you badge counts.
	LibraryOrderNeedCount(ctx context.Context) (int, error)
}

// ErrBadEpisodeOrder is an answer whose choice and order id disagree.
var ErrBadEpisodeOrder = errors.New("an episode order names an order only when it picks one")

func (s *Store) LibraryEpisodeOrderFor(ctx context.Context, series LibraryCatalogKey) (LibraryEpisodeOrder, bool, error) {
	out := LibraryEpisodeOrder{Series: series}
	var at int64
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT choice, order_id, answered_at_ms FROM library_episode_orders
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

func (s *Store) SaveLibraryEpisodeOrder(ctx context.Context, a LibraryEpisodeOrder) error {
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
	if _, err := s.h.ExecContext(ctx, s.ph(`INSERT INTO library_episode_orders
		(series_catalog, choice, order_id, answered_at_ms) VALUES (?, ?, ?, ?)
		ON CONFLICT (series_catalog) DO UPDATE SET choice = excluded.choice, order_id = excluded.order_id,
		answered_at_ms = excluded.answered_at_ms`),
		a.Series.String(), string(a.Choice), a.OrderID, a.AnsweredAt.UnixMilli()); err != nil {
		return fmt.Errorf("save library episode order: %w", err)
	}
	return nil
}

// presentEpisodeFile is episode e having a present file.
const presentEpisodeFile = `e.kind = 'episode'
	AND EXISTS (SELECT 1 FROM library_files f WHERE f.inventory_item_id = e.id AND ` + presentFile + `)`

// waitingShows is each series with an unnumbered present episode, and how many it has. It is
// driven by the partial index on unnumbered episodes, so a library where nothing waits reads none.
const waitingShows = `SELECT e.series_id AS id, COUNT(*) AS n FROM library_items e
	WHERE e.unnumbered AND e.series_id <> '' AND ` + presentEpisodeFile + ` GROUP BY e.series_id`

func (s *Store) LibraryOrderNeedCount(ctx context.Context) (int, error) {
	var n int
	if err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM library_items i
		JOIN (`+waitingShows+`) w ON w.id = i.id WHERE i.kind = 'series' AND `+itemListed)).Scan(&n); err != nil {
		return 0, fmt.Errorf("library order need count: %w", err)
	}
	return n, nil
}

func (s *Store) LibraryOrderNeeds(ctx context.Context) ([]LibraryOrderNeed, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT `+itemColumns+`, w.n
		FROM library_items i JOIN (`+waitingShows+`) w ON w.id = i.id
		WHERE i.kind = 'series' AND `+itemListed+` ORDER BY i.sort_name, i.id`))
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
	if len(out) == 0 {
		return out, nil
	}
	byID := make(map[string]*LibraryOrderNeed, len(out))
	for i := range out {
		out[i].Last, out[i].Waiting = map[int]int{}, []LibraryWaitingFile{}
		byID[out[i].ID] = &out[i]
	}
	if err := s.lastEpisodes(ctx, byID); err != nil {
		return nil, err
	}
	if err := s.waitingFiles(ctx, byID); err != nil {
		return nil, err
	}
	return out, nil
}

// waitingFiles fills each waiting show's present files of its unnumbered episodes, in path order:
// one read for every show.
func (s *Store) waitingFiles(ctx context.Context, needs map[string]*LibraryOrderNeed) error {
	// Driven from the waiting shows, which read the partial index, then each one's episodes.
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT e.series_id, f.path, e.season, e.episode
		FROM (`+waitingShows+`) w JOIN library_items e ON e.series_id = w.id
		JOIN library_files f ON f.inventory_item_id = e.id AND `+presentFile+`
		WHERE e.unnumbered AND e.kind = 'episode' ORDER BY e.series_id, f.path`))
	if err != nil {
		return fmt.Errorf("library waiting files: %w", err)
	}
	for rows.Next() {
		var series string
		var f LibraryWaitingFile
		if err := rows.Scan(&series, &f.Path, &f.Season, &f.Episode); err != nil {
			_ = rows.Close()
			return fmt.Errorf("library waiting files: %w", err)
		}
		if n := needs[series]; n != nil {
			n.Waiting = append(n.Waiting, f)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("library waiting files: %w", err)
	}
	return nil
}

// lastEpisodes fills, for each waiting show, the highest episode on disk in each of its seasons, a
// multi-episode file counted through its last episode: one read for every show.
func (s *Store) lastEpisodes(ctx context.Context, needs map[string]*LibraryOrderNeed) error {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT e.series_id, e.season, MAX(e.episode), MAX(e.episode_end)
		FROM (`+waitingShows+`) w JOIN library_items e ON e.series_id = w.id
		WHERE e.episode > 0 AND `+presentEpisodeFile+` GROUP BY e.series_id, e.season`))
	if err != nil {
		return fmt.Errorf("library last episodes: %w", err)
	}
	for rows.Next() {
		var series string
		var season, episode, end int
		if err := rows.Scan(&series, &season, &episode, &end); err != nil {
			_ = rows.Close()
			return fmt.Errorf("library last episodes: %w", err)
		}
		if n := needs[series]; n != nil {
			n.Last[season] = max(episode, end)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("library last episodes: %w", err)
	}
	return nil
}
