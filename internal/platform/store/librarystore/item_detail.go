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
	Overview      string
	OriginalTitle string
	Tagline       string
	// PremiereDate is as the source gave it, such as "2013-05-01"; "" unknown.
	PremiereDate string
	// Score is the community score, 0–10; 0 unknown.
	Score float64
	// Genres, Studios and Countries are in the source's order.
	Genres, Studios, Countries []string
	// Rating is the item's own content rating.
	Rating string
	// RuntimeMs is the catalog's runtime; each source's DurationMs is its measured length.
	RuntimeMs int64
	// Still is the image hash of the item's still (role thumb, #4476): an episode's; "" for none.
	Still string
	From  LibraryFrom
	// Credits are in billing order, each with Name, Role, Character, catalog id and PhotoHash;
	// never the source's photo URL.
	Credits []LibraryCredit
	// Sources are the item's own files, in path order; a series has none, and its episodes'
	// sources are not read. A source carries every column but SizeBytes, with its Streams in
	// index order (a sidecar track follows the file's own) and its Chapters in order; not its
	// Trickplay or Markers. A stream carries every column but SampleRate, PixelFormat,
	// ColorTransfer and Resolution, which the page does not show.
	Sources []LibrarySource
	// Episodes are, for a series, the items its present files were written as, by item id: its
	// episodes, or whatever else a file under it was written as. Any other item has none.
	Episodes map[string]LibraryDetailEpisode
}

// LibraryDetailEpisode is one episode as its series' page lists it.
type LibraryDetailEpisode struct {
	Name string
	// NameFrom is the provider its name came from, "" for its file's.
	NameFrom string
	LibraryEpisodeNumbers
	// Unnumbered keeps its file's name and number until its show's order is said (decision 53).
	Unnumbered bool
	// Overview is the episode's own story.
	Overview string
	// LengthMs is the measured length of the files that play, once each is read, else the
	// catalog's runtime; 0 when neither is known. Only the sources' durations are read.
	LengthMs int64
	// Still is the image hash its still's ref names (role thumb, #4476); "" for none.
	Still string
}

// LibraryEpisodeNumbers numbers an episode: EpisodeEnd is the last episode a multi-episode file
// holds, 0 for one.
type LibraryEpisodeNumbers struct {
	Season, Episode, EpisodeEnd int
}

// LibraryItemDetailReader reads one title's page.
type LibraryItemDetailReader interface {
	// LibraryItemDetail reads the item id names, in one read transaction; an id with no row reads
	// zero.
	LibraryItemDetail(ctx context.Context, id string) (LibraryItemDetail, error)
	// LibraryEpisodeNumbers reads the numbers of the items a series' present files were written
	// as, by item id, as LibraryItemDetail's Episodes holds them. Any other id has none.
	LibraryEpisodeNumbers(ctx context.Context, seriesID string) (map[string]LibraryEpisodeNumbers, error)
}

func (s *Store) LibraryItemDetail(ctx context.Context, id string) (LibraryItemDetail, error) {
	tx, err := s.h.BeginReadOnly(ctx)
	if err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	var out LibraryItemDetail
	err = tx.QueryRowContext(ctx, s.ph(`SELECT overview, original_title, tagline, premiere_date, score, rating,
		runtime_ms, name_from, year_from, overview_from, premiere_from, rating_from, score_from, runtime_from,
		`+itemImage("thumb")+`
		FROM library_items i WHERE id = ?`), id).
		Scan(&out.Overview, &out.OriginalTitle, &out.Tagline, &out.PremiereDate, &out.Score, &out.Rating,
			&out.RuntimeMs, &out.From.Name, &out.From.Year, &out.From.Overview, &out.From.Premiere,
			&out.From.Rating, &out.From.Score, &out.From.Runtime, &out.Still)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	for _, read := range []func() error{
		func() (err error) { out.Genres, err = s.detailNames(ctx, tx, "library_genres", id); return err },
		func() (err error) { out.Studios, err = s.detailNames(ctx, tx, "library_studios", id); return err },
		func() (err error) { out.Countries, err = s.detailNames(ctx, tx, "library_countries", id); return err },
		func() (err error) { out.Credits, err = s.detailCredits(ctx, tx, id); return err },
		func() (err error) { out.Sources, err = s.detailSources(ctx, tx, id); return err },
		func() (err error) { out.Episodes, err = s.detailEpisodes(ctx, tx, id); return err },
	} {
		if err = read(); err != nil {
			break
		}
	}
	if err != nil {
		return LibraryItemDetail{}, fmt.Errorf("library item detail %s: %w", id, err)
	}
	return out, tx.Commit()
}

func (s *Store) LibraryEpisodeNumbers(ctx context.Context, seriesID string) (map[string]LibraryEpisodeNumbers, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT DISTINCT i.id, i.season, i.episode, i.episode_end
		FROM library_files f JOIN library_items i ON i.id = f.inventory_item_id
		WHERE f.inventory_series_id = ? AND `+presentFile), seriesID)
	if err != nil {
		return nil, fmt.Errorf("library episode numbers %s: %w", seriesID, err)
	}
	out := map[string]LibraryEpisodeNumbers{}
	for rows.Next() {
		var id string
		var n LibraryEpisodeNumbers
		if err := rows.Scan(&id, &n.Season, &n.Episode, &n.EpisodeEnd); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("library episode numbers %s: %w", seriesID, err)
		}
		out[id] = n
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("library episode numbers %s: %w", seriesID, err)
	}
	return out, nil
}

// detailNames reads an item's names from table, one of library_genres, library_studios and
// library_countries, in position order.
func (s *Store) detailNames(ctx context.Context, q storedb.Querier, table, id string) ([]string, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT name FROM `+table+` WHERE item_id = ? ORDER BY position`), id)
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

// detailCredits reads an item's credits in billing order, each with its person's name and
// catalog id, so PersonID names the same library_people row.
func (s *Store) detailCredits(ctx context.Context, q storedb.Querier, id string) ([]LibraryCredit, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT p.name, c.role, c.character_name, p.namespace, p.external_id,
		COALESCE((SELECT MIN(r.image_hash) FROM library_person_images r WHERE r.person_id = p.id), '')
		FROM library_credits c
		JOIN library_people p ON p.id = c.person_id WHERE c.item_id = ? ORDER BY c.position`), id)
	if err != nil {
		return nil, err
	}
	var out []LibraryCredit
	for rows.Next() {
		var c LibraryCredit
		if err := rows.Scan(&c.Name, &c.Role, &c.Character, &c.ID.Namespace, &c.ID.Value, &c.PhotoHash); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	return out, errors.Join(rows.Err(), rows.Close())
}

// detailEpisodes joins from the series' present files (library_files' inventory_series_id index)
// to the items they were written as. None of them is the series: a file's item is the movie or
// episode its own file origin writes, and the inventory joins ids only within one kind, so a file
// is never written as a series. An episode's length is the title list's (libraryItemFacts): the
// sum of the playing edition's sources that play once each is measured, keyed by item_id, else
// its runtime_ms.
func (s *Store) detailEpisodes(ctx context.Context, q storedb.Querier, seriesID string) (map[string]LibraryDetailEpisode, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT DISTINCT i.id, i.name, i.name_from, i.season, i.episode,
		i.episode_end, i.unnumbered, i.overview,
		COALESCE(NULLIF((SELECT CAST(CASE WHEN MIN(src.duration_ms) > 0 THEN SUM(src.duration_ms) ELSE 0 END AS BIGINT)
			FROM library_sources src WHERE src.item_id = i.id AND src.plays AND src.edition = (SELECT e.edition
			FROM library_sources e WHERE e.item_id = i.id AND e.plays ORDER BY e.edition <> '', e.path LIMIT 1)), 0), i.runtime_ms),
		`+itemImage("thumb")+`
		FROM library_files f JOIN library_items i ON i.id = f.inventory_item_id
		WHERE f.inventory_series_id = ? AND `+presentFile), seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]LibraryDetailEpisode{}
	for rows.Next() {
		var id string
		var e LibraryDetailEpisode
		if err := rows.Scan(&id, &e.Name, &e.NameFrom, &e.Season, &e.Episode, &e.EpisodeEnd, &e.Unnumbered,
			&e.Overview, &e.LengthMs, &e.Still); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out[id] = e
	}
	return out, errors.Join(rows.Err(), rows.Close())
}

// detailSources reads the item's sources, their streams and their chapters by their keys' leading
// item_id.
func (s *Store) detailSources(ctx context.Context, q storedb.Querier, id string) ([]LibrarySource, error) {
	rows, err := q.QueryContext(ctx, s.ph(`SELECT path, container, bitrate, duration_ms, edition, part, plays
		FROM library_sources WHERE item_id = ? ORDER BY path`), id)
	if err != nil {
		return nil, err
	}
	var out []LibrarySource
	at := map[string]int{}
	for rows.Next() {
		var src LibrarySource
		if err := rows.Scan(&src.Path, &src.Container, &src.Bitrate, &src.DurationMs, &src.Edition, &src.Part,
			&src.Plays); err != nil {
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
	rows, err = q.QueryContext(ctx, s.ph(`SELECT path, stream_index, kind, codec, profile, title, is_default,
		forced, language, language_name, channels, channel_layout, audio_format, width, height, frame_rate, hdr,
		commentary, hearing_impaired, descriptive, external_ref, bitrate, bit_depth
		FROM library_streams WHERE item_id = ? ORDER BY path, stream_index`), id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path string
		var st LibraryStream
		if err := rows.Scan(&path, &st.Index, &st.Kind, &st.Codec, &st.Profile, &st.Title, &st.Default, &st.Forced,
			&st.Language, &st.LanguageName, &st.Channels, &st.ChannelLayout, &st.AudioFormat, &st.Width, &st.Height,
			&st.FrameRate, &st.HDR, &st.Commentary, &st.HearingImpaired, &st.Descriptive, &st.ExternalRef,
			&st.BitRate, &st.BitDepth); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if i, ok := at[path]; ok {
			out[i].Streams = append(out[i].Streams, st)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, s.ph(`SELECT path, start_ms, end_ms, title FROM library_chapters
		WHERE item_id = ? ORDER BY path, position`), id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path string
		var c LibraryChapter
		if err := rows.Scan(&path, &c.StartMs, &c.EndMs, &c.Title); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if i, ok := at[path]; ok {
			out[i].Chapters = append(out[i].Chapters, c)
		}
	}
	return out, errors.Join(rows.Err(), rows.Close())
}
