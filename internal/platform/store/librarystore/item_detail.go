package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// LibraryItemDetail is what a title's page reads about one library item from the Library's own
// rows (#4476): its facts and where they came from, its files' rows with their tracks, and for a
// series each episode that has a present file.
type LibraryItemDetail struct {
	Overview string
	// Genres are in the source's order.
	Genres []string
	// Rating is the item's own content rating.
	Rating string
	// RuntimeMs is the catalog's runtime; each source's DurationMs is its measured length.
	RuntimeMs int64
	From      LibraryFrom
	// Sources are the item's files and, for a series, its episodes' files, by the item that lists
	// them, each in path order. A source carries Path, DurationMs, Edition, Part, Plays and its
	// Streams; each stream carries Index, Kind, Width, Height, HDR, Channels, Default, Language and
	// ExternalRef, in index order, so a sidecar track follows the file's own.
	Sources map[string][]LibrarySource
	// Episodes are the items a series' present files were written as, by item id: its episodes, or
	// whatever else a file under it was written as.
	Episodes map[string]LibraryDetailEpisode
}

// LibraryDetailEpisode is one episode as its series' page lists it.
type LibraryDetailEpisode struct {
	Name string
	// NameFrom is the provider its name came from, "" for its file's.
	NameFrom                    string
	Season, Episode, EpisodeEnd int
	// Unnumbered keeps its file's name and number until its show's order is said (decision 53).
	Unnumbered bool
}

// LibraryItemDetailReader reads one title's page.
type LibraryItemDetailReader interface {
	// LibraryItemDetail reads the item id names; an id with no row reads zero.
	LibraryItemDetail(ctx context.Context, id string) (LibraryItemDetail, error)
}

// detailItems is the item ?1 and the episodes ?2 its present files name as their series, as a
// condition on a column holding an item id.
func detailItems(column string) string {
	return `(` + column + ` = ? OR ` + column + ` IN (SELECT f.inventory_item_id FROM library_files f
		WHERE f.inventory_series_id = ? AND ` + presentFile + `))`
}

func (s *Store) LibraryItemDetail(ctx context.Context, id string) (LibraryItemDetail, error) {
	out := LibraryItemDetail{Sources: map[string][]LibrarySource{}, Episodes: map[string]LibraryDetailEpisode{}}
	err := s.h.QueryRowContext(ctx, s.ph(`SELECT overview, rating, runtime_ms, name_from, year_from,
		overview_from, premiere_from, rating_from, score_from, runtime_from FROM library_items WHERE id = ?`), id).
		Scan(&out.Overview, &out.Rating, &out.RuntimeMs, &out.From.Name, &out.From.Year, &out.From.Overview,
			&out.From.Premiere, &out.From.Rating, &out.From.Score, &out.From.Runtime)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	for _, read := range []func(context.Context, string, *LibraryItemDetail) error{
		s.detailGenres, s.detailEpisodes, s.detailSources, s.detailStreams,
	} {
		if err := read(ctx, id, &out); err != nil {
			return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
		}
	}
	return out, nil
}

func (s *Store) detailGenres(ctx context.Context, id string, out *LibraryItemDetail) error {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT name FROM library_genres WHERE item_id = ? ORDER BY position`), id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		out.Genres = append(out.Genres, name)
	}
	return errors.Join(rows.Err(), rows.Close())
}

func (s *Store) detailEpisodes(ctx context.Context, id string, out *LibraryItemDetail) error {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT i.id, i.name, i.name_from, i.season, i.episode, i.episode_end,
		i.unnumbered FROM library_items i WHERE i.id <> ? AND `+detailItems("i.id")), id, id, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var epID string
		var e LibraryDetailEpisode
		if err := rows.Scan(&epID, &e.Name, &e.NameFrom, &e.Season, &e.Episode, &e.EpisodeEnd, &e.Unnumbered); err != nil {
			_ = rows.Close()
			return err
		}
		out.Episodes[epID] = e
	}
	return errors.Join(rows.Err(), rows.Close())
}

func (s *Store) detailSources(ctx context.Context, id string, out *LibraryItemDetail) error {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT s.item_id, s.path, s.duration_ms, s.edition, s.part, s.plays
		FROM library_sources s WHERE `+detailItems("s.item_id")+` ORDER BY s.item_id, s.path`), id, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var itemID string
		var src LibrarySource
		if err := rows.Scan(&itemID, &src.Path, &src.DurationMs, &src.Edition, &src.Part, &src.Plays); err != nil {
			_ = rows.Close()
			return err
		}
		out.Sources[itemID] = append(out.Sources[itemID], src)
	}
	return errors.Join(rows.Err(), rows.Close())
}

func (s *Store) detailStreams(ctx context.Context, id string, out *LibraryItemDetail) error {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT st.item_id, st.path, st.stream_index, st.kind, st.width,
		st.height, st.hdr, st.channels, st.is_default, st.language, st.external_ref
		FROM library_streams st WHERE `+detailItems("st.item_id")+` ORDER BY st.item_id, st.path, st.stream_index`), id, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var itemID, path string
		var st LibraryStream
		if err := rows.Scan(&itemID, &path, &st.Index, &st.Kind, &st.Width, &st.Height, &st.HDR, &st.Channels,
			&st.Default, &st.Language, &st.ExternalRef); err != nil {
			_ = rows.Close()
			return err
		}
		sources := out.Sources[itemID]
		for i := range sources {
			if sources[i].Path == path {
				sources[i].Streams = append(sources[i].Streams, st)
				break
			}
		}
	}
	return errors.Join(rows.Err(), rows.Close())
}
