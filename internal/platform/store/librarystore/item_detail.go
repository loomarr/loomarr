package librarystore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// LibraryItemDetail is what a title's page reads about one library item from the Library's own
// rows (#4476): its facts and where they came from, its own files' rows with their tracks, and for
// a series the items its present files were written as.
type LibraryItemDetail struct {
	Overview string
	// Genres are in the source's order.
	Genres []string
	// Rating is the item's own content rating.
	Rating string
	// RuntimeMs is the catalog's runtime; each source's DurationMs is its measured length.
	RuntimeMs int64
	From      LibraryFrom
	// Sources are the item's own files, in path order; a series has none, and its episodes'
	// sources are not read. A source carries Path, DurationMs, Edition, Part, Plays and its
	// Streams; each stream carries Index, Kind, Width, Height, HDR, Channels, Default, Language and
	// ExternalRef, in index order, so a sidecar track follows the file's own.
	Sources []LibrarySource
	// Episodes are what LibraryDetailEpisodes reads for the item.
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
	// LibraryItemDetail reads the item id names, in one read transaction; an id with no row reads
	// zero.
	LibraryItemDetail(ctx context.Context, id string) (LibraryItemDetail, error)
	// LibraryDetailEpisodes reads the items a series' present files were written as, by item id:
	// its episodes, or whatever else a file under it was written as, never the series itself. Any
	// other id has none.
	LibraryDetailEpisodes(ctx context.Context, seriesID string) (map[string]LibraryDetailEpisode, error)
}

func (s *Store) LibraryItemDetail(ctx context.Context, id string) (LibraryItemDetail, error) {
	tx, err := s.h.BeginReadOnly(ctx)
	if err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	var out LibraryItemDetail
	err = tx.QueryRowContext(ctx, s.ph(`SELECT overview, rating, runtime_ms, name_from, year_from,
		overview_from, premiere_from, rating_from, score_from, runtime_from FROM library_items WHERE id = ?`), id).
		Scan(&out.Overview, &out.Rating, &out.RuntimeMs, &out.From.Name, &out.From.Year, &out.From.Overview,
			&out.From.Premiere, &out.From.Rating, &out.From.Score, &out.From.Runtime)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	if out.Genres, err = s.detailGenres(ctx, tx, id); err == nil {
		if out.Sources, err = s.detailSources(ctx, tx, id); err == nil {
			out.Episodes, err = s.detailEpisodes(ctx, tx, id)
		}
	}
	if err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	return out, tx.Commit()
}

func (s *Store) LibraryDetailEpisodes(ctx context.Context, seriesID string) (map[string]LibraryDetailEpisode, error) {
	eps, err := s.detailEpisodes(ctx, s.h, seriesID)
	if err != nil {
		return nil, fmt.Errorf("library detail episodes %s: %w", seriesID, err)
	}
	return eps, nil
}

func (s *Store) detailGenres(ctx context.Context, q storedb.Querier, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT name FROM library_genres WHERE item_id = ? ORDER BY position`), id)
	if err != nil {
		return nil, err
	}
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, name)
	}
	return out, errors.Join(rows.Err(), rows.Close())
}

// detailEpisodes joins from the series' present files (library_files' inventory_series_id index)
// to the items they were written as, other than the series itself.
func (s *Store) detailEpisodes(ctx context.Context, q storedb.Querier, seriesID string) (map[string]LibraryDetailEpisode, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT DISTINCT i.id, i.name, i.name_from, i.season, i.episode,
		i.episode_end, i.unnumbered FROM library_files f JOIN library_items i ON i.id = f.inventory_item_id
		WHERE f.inventory_series_id = ? AND f.inventory_item_id <> ? AND `+presentFile), seriesID, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]LibraryDetailEpisode{}
	for rows.Next() {
		var id string
		var e LibraryDetailEpisode
		if err := rows.Scan(&id, &e.Name, &e.NameFrom, &e.Season, &e.Episode, &e.EpisodeEnd, &e.Unnumbered); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out[id] = e
	}
	return out, errors.Join(rows.Err(), rows.Close())
}

// detailSources reads the item's sources and their streams by their keys' leading item_id.
func (s *Store) detailSources(ctx context.Context, q storedb.Querier, id string) ([]LibrarySource, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT path, duration_ms, edition, part, plays
		FROM library_sources WHERE item_id = ? ORDER BY path`), id)
	if err != nil {
		return nil, err
	}
	var out []LibrarySource
	at := map[string]int{}
	for rows.Next() {
		var src LibrarySource
		if err := rows.Scan(&src.Path, &src.DurationMs, &src.Edition, &src.Part, &src.Plays); err != nil {
			_ = rows.Close()
			return nil, err
		}
		at[src.Path] = len(out)
		out = append(out, src)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	rows, err = q.QueryContext(ctx, s.ph(`SELECT path, stream_index, kind, width, height, hdr, channels, is_default,
		language, external_ref FROM library_streams WHERE item_id = ? ORDER BY path, stream_index`), id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path string
		var st LibraryStream
		if err := rows.Scan(&path, &st.Index, &st.Kind, &st.Width, &st.Height, &st.HDR, &st.Channels, &st.Default,
			&st.Language, &st.ExternalRef); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if i, ok := at[path]; ok {
			out[i].Streams = append(out[i].Streams, st)
		}
	}
	return out, errors.Join(rows.Err(), rows.Close())
}
