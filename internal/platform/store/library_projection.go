package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// LibraryProjector says how an inventory item is listed in the Library (#2151 decision 43): its
// library_items row, or false when it has none. It is pure, and the scanner supplies it, since
// only the scanner can read what it wrote in an origin's extension.
type LibraryProjector func(inventory.Item) (LibraryItem, bool)

// LibraryProjection is how the Library's rows are kept in step with the inventory.
type LibraryProjection interface {
	// ProjectLibraryWith installs the projector. From then on each inventory write that changes
	// what it reads (a snapshot, an origin marked unseen) rewrites the items it touched in its own
	// transaction, whoever writes. Before it is installed nothing is projected.
	ProjectLibraryWith(LibraryProjector)
}

func (s *sqlStore) ProjectLibraryWith(p LibraryProjector) { s.libraryProjector.Store(&p) }

// projectLibrary rewrites the library rows of each item in tx. An item the inventory no longer
// has, or one the projector does not list, keeps whatever row it had: presence comes from
// library_files, and a title that comes back finds its row, its collections and its answer.
func (s *sqlStore) projectLibrary(ctx context.Context, tx *sql.Tx, at time.Time, ids ...inventory.ItemID) error {
	p := s.libraryProjector.Load()
	if p == nil || *p == nil {
		return nil
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		item, ok, err := s.readInventoryItem(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("project library item %s: %w", id, err)
		}
		if !ok {
			continue
		}
		row, ok := (*p)(item)
		if !ok {
			continue
		}
		if err := s.saveLibraryItem(ctx, tx, row, at); err != nil {
			return err
		}
	}
	return nil
}

// saveLibraryItem writes one item whole: its library_items row and its ids, genres, studios and
// tags. An episode is linked to its series, and a series to its episodes, whichever is written
// second (decision 42).
func (s *sqlStore) saveLibraryItem(ctx context.Context, tx *sql.Tx, it LibraryItem, at time.Time) error {
	if it.ID == "" || it.Kind == "" {
		return fmt.Errorf("save library item: id and kind are required")
	}
	sortName := it.SortName
	if sortName == "" {
		sortName = strings.ToLower(it.Name)
	}
	if it.Kind == "episode" {
		if err := s.withSeries(ctx, tx, &it); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_items (id, kind, name, sort_name, year, catalog,
		origin_authority, origin_key, grounded, match_basis, match_app, match_page, set_key, updated_at_ms,
		original_title, overview, premiere_date, rating, score, runtime_ms, season, episode, episode_end,
		series_authority, series_key, series_id,
		name_from, year_from, overview_from, premiere_from, rating_from, score_from, runtime_from)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, name = excluded.name,
		sort_name = excluded.sort_name, year = excluded.year, catalog = excluded.catalog,
		origin_authority = excluded.origin_authority, origin_key = excluded.origin_key,
		grounded = excluded.grounded, match_basis = excluded.match_basis, match_app = excluded.match_app,
		match_page = excluded.match_page, set_key = excluded.set_key, updated_at_ms = excluded.updated_at_ms,
		original_title = excluded.original_title, overview = excluded.overview,
		premiere_date = excluded.premiere_date, rating = excluded.rating, score = excluded.score,
		runtime_ms = excluded.runtime_ms, season = excluded.season, episode = excluded.episode,
		episode_end = excluded.episode_end, series_authority = excluded.series_authority,
		series_key = excluded.series_key, series_id = excluded.series_id,
		name_from = excluded.name_from, year_from = excluded.year_from,
		overview_from = excluded.overview_from, premiere_from = excluded.premiere_from,
		rating_from = excluded.rating_from, score_from = excluded.score_from,
		runtime_from = excluded.runtime_from`),
		it.ID, it.Kind, it.Name, sortName, it.Year, it.Catalog.String(), it.Origin.Authority, it.Origin.Key,
		it.Match.Grounded(), it.Match.Basis, it.Match.App, it.Match.Page, it.Set.String(), at.UnixMilli(),
		it.OriginalTitle, it.Overview, it.PremiereDate, it.Rating, it.Score, it.RuntimeMs,
		it.Season, it.Episode, it.EpisodeEnd, it.Series.Authority, it.Series.Key, it.SeriesID,
		it.From.Name, it.From.Year, it.From.Overview, it.From.Premiere, it.From.Rating, it.From.Score,
		it.From.Runtime); err != nil {
		return fmt.Errorf("save library item: %w", err)
	}
	if err := s.saveLibraryItemRows(ctx, tx, it); err != nil {
		return err
	}
	if it.Kind == "series" {
		return s.linkEpisodes(ctx, tx, it)
	}
	return nil
}

// saveLibraryItemRows replaces an item's ids, genres, studios and tags.
func (s *sqlStore) saveLibraryItemRows(ctx context.Context, tx *sql.Tx, it LibraryItem) error {
	for _, table := range []string{"library_ids", "library_genres", "library_studios", "library_tags"} {
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM `+table+` WHERE item_id = ?`), it.ID); err != nil {
			return fmt.Errorf("replace %s: %w", table, err)
		}
	}
	seen := map[LibraryID]bool{}
	for _, id := range it.IDs {
		if id.Namespace == "" || id.Value == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_ids (item_id, namespace, value) VALUES (?, ?, ?)`),
			it.ID, id.Namespace, id.Value); err != nil {
			return fmt.Errorf("save library id: %w", err)
		}
	}
	for table, names := range map[string][]string{"library_genres": it.Genres, "library_studios": it.Studios, "library_tags": it.Tags} {
		seenName := map[string]bool{}
		for i, name := range names {
			if name == "" || seenName[name] {
				continue
			}
			seenName[name] = true
			if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO `+table+` (item_id, name, position) VALUES (?, ?, ?)`),
				it.ID, name, i); err != nil {
				return fmt.Errorf("save %s: %w", table, err)
			}
		}
	}
	return nil
}

// withSeries fills an episode's SeriesID from its series' origin, when the series is written, and
// gives an episode with no rating its show's.
func (s *sqlStore) withSeries(ctx context.Context, tx *sql.Tx, it *LibraryItem) error {
	it.SeriesID = ""
	if it.Series.Key == "" {
		return nil
	}
	var seriesID, rating string
	err := tx.QueryRowContext(ctx, s.ph(`SELECT s.id, s.rating FROM inventory_item_origins o
		JOIN library_items s ON s.id = o.item_id AND s.kind = 'series'
		WHERE o.authority_id = ? AND o.external_item_id = ?`), it.Series.Authority, it.Series.Key).Scan(&seriesID, &rating)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read episode's series: %w", err)
	}
	it.SeriesID = seriesID
	if it.Rating == "" && rating != "" {
		it.Rating, it.From.Rating = rating, RatingFromShow
	}
	return nil
}

// linkEpisodes points every episode that names one of the series' origins at it, and gives those
// with no rating of their own the show's.
func (s *sqlStore) linkEpisodes(ctx context.Context, tx *sql.Tx, it LibraryItem) error {
	from := ""
	if it.Rating != "" {
		from = RatingFromShow
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE library_items SET series_id = ?,
		rating = CASE WHEN rating = '' OR rating_from = ? THEN ? ELSE rating END,
		rating_from = CASE WHEN rating = '' OR rating_from = ? THEN ? ELSE rating_from END
		WHERE kind = 'episode' AND EXISTS (SELECT 1 FROM inventory_item_origins o WHERE o.item_id = ?
			AND o.authority_id = library_items.series_authority AND o.external_item_id = library_items.series_key)`),
		it.ID, RatingFromShow, it.Rating, RatingFromShow, from, it.ID); err != nil {
		return fmt.Errorf("link series episodes: %w", err)
	}
	return nil
}
