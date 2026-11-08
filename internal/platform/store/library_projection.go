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
		name_from, year_from, overview_from, premiere_from, rating_from, score_from, runtime_from,
		tagline, trailer)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		runtime_from = excluded.runtime_from, tagline = excluded.tagline, trailer = excluded.trailer`),
		it.ID, it.Kind, it.Name, sortName, it.Year, it.Catalog.String(), it.Origin.Authority, it.Origin.Key,
		it.Match.Grounded(), it.Match.Basis, it.Match.App, it.Match.Page, it.Set.String(), at.UnixMilli(),
		it.OriginalTitle, it.Overview, it.PremiereDate, it.Rating, it.Score, it.RuntimeMs,
		it.Season, it.Episode, it.EpisodeEnd, it.Series.Authority, it.Series.Key, it.SeriesID,
		it.From.Name, it.From.Year, it.From.Overview, it.From.Premiere, it.From.Rating, it.From.Score,
		it.From.Runtime, it.Tagline, it.Trailer); err != nil {
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

// saveLibraryItemRows replaces an item's ids, genres, studios, tags, files, tracks and artwork.
func (s *sqlStore) saveLibraryItemRows(ctx context.Context, tx *sql.Tx, it LibraryItem) error {
	for _, table := range []string{"library_trickplay", "library_chapters", "library_markers", "library_streams",
		"library_sources", "library_art",
		"library_ids", "library_genres", "library_studios", "library_countries", "library_tags", "library_credits"} {
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
	for table, names := range map[string][]string{"library_genres": it.Genres, "library_studios": it.Studios,
		"library_countries": it.Countries, "library_tags": it.Tags} {
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
	if err := s.saveLibraryCredits(ctx, tx, it); err != nil {
		return err
	}
	return s.saveLibraryFilesAndArt(ctx, tx, it)
}

// saveLibraryCredits writes an item's credits, and each person it names. A person's name and
// photo are the latest any title gave; a title without a photo keeps the one held.
func (s *sqlStore) saveLibraryCredits(ctx context.Context, tx *sql.Tx, it LibraryItem) error {
	seen := map[string]bool{}
	for i, c := range it.Credits {
		if c.Name == "" || c.Role == "" {
			continue
		}
		id := c.PersonID()
		key := id + "\x00" + c.Role + "\x00" + c.Character
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_people (id, name, tmdb_id, photo) VALUES (?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name,
			photo = CASE WHEN excluded.photo <> '' THEN excluded.photo ELSE library_people.photo END`),
			id, c.Name, c.TMDB, c.Photo); err != nil {
			return fmt.Errorf("save library person: %w", err)
		}
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_credits (item_id, position, person_id, role,
			character_name) VALUES (?, ?, ?, ?, ?)`), it.ID, i, id, c.Role, c.Character); err != nil {
			return fmt.Errorf("save library credit: %w", err)
		}
	}
	return nil
}

// saveLibraryFilesAndArt writes an item's files with their tracks, and its artwork.
func (s *sqlStore) saveLibraryFilesAndArt(ctx context.Context, tx *sql.Tx, it LibraryItem) error {
	paths := map[string]bool{}
	for _, src := range it.Sources {
		if src.Path == "" || paths[src.Path] {
			continue
		}
		paths[src.Path] = true
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_sources (item_id, path, container, size_bytes,
			bitrate, duration_ms, edition, plays) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
			it.ID, src.Path, src.Container, src.SizeBytes, src.Bitrate, src.DurationMs, src.Edition, src.Plays); err != nil {
			return fmt.Errorf("save library source: %w", err)
		}
		refs := map[string]bool{}
		for _, tp := range src.Trickplay {
			if tp.Ref == "" || refs[tp.Ref] {
				continue
			}
			refs[tp.Ref] = true
			if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_trickplay (item_id, path, ref, format)
				VALUES (?, ?, ?, ?)`), it.ID, src.Path, tp.Ref, tp.Format); err != nil {
				return fmt.Errorf("save library trickplay: %w", err)
			}
		}
		indexes := map[int]bool{}
		for _, st := range src.Streams {
			if indexes[st.Index] {
				continue
			}
			indexes[st.Index] = true
			if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_streams (item_id, path, stream_index, kind,
				codec, profile, title, is_default, forced, language, language_name, channels, channel_layout,
				sample_rate, audio_format, width, height, frame_rate, pixel_format, color_transfer, resolution, hdr,
				commentary, hearing_impaired, descriptive, external_ref)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
				it.ID, src.Path, st.Index, st.Kind, st.Codec, st.Profile, st.Title, st.Default, st.Forced,
				st.Language, st.LanguageName, st.Channels, st.ChannelLayout, st.SampleRate, st.AudioFormat,
				st.Width, st.Height, st.FrameRate, st.PixelFormat, st.ColorTransfer, st.Resolution, st.HDR,
				st.Commentary, st.HearingImpaired, st.Descriptive, st.ExternalRef); err != nil {
				return fmt.Errorf("save library stream: %w", err)
			}
		}
		for i, c := range src.Chapters {
			if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_chapters (item_id, path, position, start_ms,
				end_ms, title) VALUES (?, ?, ?, ?, ?, ?)`), it.ID, src.Path, i, c.StartMs, c.EndMs, c.Title); err != nil {
				return fmt.Errorf("save library chapter: %w", err)
			}
		}
		for i, m := range src.Markers {
			if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_markers (item_id, path, position, start_ms,
				end_ms, kind) VALUES (?, ?, ?, ?, ?, ?)`), it.ID, src.Path, i, m.StartMs, m.EndMs, m.Kind); err != nil {
				return fmt.Errorf("save library marker: %w", err)
			}
		}
	}
	positions := map[[2]any]int{}
	for _, a := range it.Art {
		if a.Kind == "" || a.Ref == "" {
			continue
		}
		key := [2]any{a.Kind, a.Season}
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_art (item_id, kind, season, position, ref)
			VALUES (?, ?, ?, ?, ?)`), it.ID, a.Kind, a.Season, positions[key], a.Ref); err != nil {
			return fmt.Errorf("save library art: %w", err)
		}
		positions[key]++
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
		WHERE kind = 'episode' AND (series_authority, series_key) IN
			(SELECT authority_id, external_item_id FROM inventory_item_origins WHERE item_id = ?)`),
		it.ID, RatingFromShow, it.Rating, RatingFromShow, from, it.ID); err != nil {
		return fmt.Errorf("link series episodes: %w", err)
	}
	return nil
}
