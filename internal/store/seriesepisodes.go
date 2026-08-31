package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/episodeevidence"
	"github.com/loomarr/loomarr/internal/schedule"
)

// SeriesEpisodes is one show's cached episode list (§5, §9 series expansion).
//
// A MATERIALIZED ANSWER, never a second source of truth: the media server still owns what
// episodes exist. This exists because enumerating them is one call PER SHOW and that call sat
// on the request path — a 4-show channel spent 232ms there on every `GET /v1/guide`, roughly
// 90% of the guide's latency.
//
// Keyed by the media server's SHOW item id rather than by provision.Key, because the same show
// can be reached through a TMDB-keyed or a TVDB-keyed lineup entry and both resolve to one
// library id. One row per show, however it was referenced.
type SeriesEpisodes struct {
	// LibraryID is the media-server SHOW item id.
	LibraryID string
	// Episodes is the resolved list in the shape the scheduler consumes. An EMPTY list is a
	// legitimate cached answer — a show whose episodes are not present yet — so the absence of
	// a ROW is the only "unknown".
	Episodes []schedule.ResolvedProgram
	// FetchedAt is when the list was last read from the library. Drives staleness
	// (`episodes.max_age`) and is what the refresh job selects on. Zero = never.
	FetchedAt time.Time
}

const seriesEpisodesSelect = `SELECT library_id, episodes_json, fetched_at FROM series_episodes`

// GetSeriesEpisodes reads one show's cached episode list.
//
// Returns ErrNotFound when the show has never been enumerated — deliberately distinct from a
// cached EMPTY list, which is a real answer ("this show has no episodes present"). Collapsing
// the two would make a genuinely-empty show re-enumerate on every single request, which is the
// N+1 this cache exists to remove.
func (s *sqlStore) GetSeriesEpisodes(ctx context.Context, libraryID string) (SeriesEpisodes, error) {
	row := s.db.QueryRowContext(ctx, s.ph(seriesEpisodesSelect+` WHERE library_id = ?`), libraryID)

	var out SeriesEpisodes
	var epsJSON string
	var fetched int64
	if err := row.Scan(&out.LibraryID, &epsJSON, &fetched); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SeriesEpisodes{}, ErrNotFound
		}
		return SeriesEpisodes{}, fmt.Errorf("get series episodes %s: %w", libraryID, err)
	}
	episodes, err := decodeSeriesEpisodes([]byte(epsJSON))
	if err != nil {
		return SeriesEpisodes{}, fmt.Errorf("decode series episodes %s: %w", libraryID, err)
	}
	out.Episodes = episodes
	if fetched > 0 {
		out.FetchedAt = time.Unix(fetched, 0)
	}
	return out, nil
}

// UpsertSeriesEpisodes writes a show's episode list, replacing whatever was cached.
//
// Whole-list replace rather than a merge: the library's answer IS the truth for that show, so
// merging would preserve episodes the media server no longer reports — a deleted episode would
// linger in every channel's lineup until someone noticed.
func (s *sqlStore) UpsertSeriesEpisodes(ctx context.Context, se SeriesEpisodes) error {
	for i, episode := range se.Episodes {
		if err := validateCachedEpisode(episode); err != nil {
			return fmt.Errorf("upsert series episodes %s: episode %d: %w", se.LibraryID, i, err)
		}
	}
	eps := sanitizeSeriesEpisodes(se.Episodes)
	if eps == nil {
		eps = []schedule.ResolvedProgram{} // store `[]`, never `null` — see the column default
	}
	blob, err := json.Marshal(eps)
	if err != nil {
		return fmt.Errorf("encode series episodes %s: %w", se.LibraryID, err)
	}
	var fetched int64
	if !se.FetchedAt.IsZero() {
		fetched = se.FetchedAt.Unix()
	}

	_, err = s.db.ExecContext(ctx, s.ph(`
		INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(library_id) DO UPDATE SET
			episodes_json = excluded.episodes_json,
			episode_count = excluded.episode_count,
			fetched_at    = excluded.fetched_at`),
		se.LibraryID, string(blob), len(eps), fetched)
	if err != nil {
		return fmt.Errorf("upsert series episodes %s: %w", se.LibraryID, err)
	}
	return nil
}

// ListStaleSeriesEpisodes returns the shows whose cached lists were fetched before `before`,
// oldest first, capped at `limit`.
//
// Oldest-first so a refresh run that hits its cap makes progress rather than re-visiting the
// same shows: the ones it skips are newer than the ones it took, so the next run picks them up.
func (s *sqlStore) ListStaleSeriesEpisodes(ctx context.Context, before time.Time, limit int) ([]SeriesEpisodes, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, s.ph(seriesEpisodesSelect+`
		WHERE fetched_at < ? ORDER BY fetched_at ASC LIMIT ?`), before.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("list stale series episodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SeriesEpisodes
	for rows.Next() {
		var se SeriesEpisodes
		var epsJSON string
		var fetched int64
		if err := rows.Scan(&se.LibraryID, &epsJSON, &fetched); err != nil {
			return nil, fmt.Errorf("scan stale series episodes: %w", err)
		}
		// A decode failure on ONE row must not fail the sweep: the row is re-fetched
		// anyway, which is exactly the repair. Leaving Episodes nil is honest.
		se.Episodes, _ = decodeSeriesEpisodes([]byte(epsJSON))
		if fetched > 0 {
			se.FetchedAt = time.Unix(fetched, 0)
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

func sanitizeSeriesEpisodes(episodes []schedule.ResolvedProgram) []schedule.ResolvedProgram {
	if episodes == nil {
		return nil
	}
	out := make([]schedule.ResolvedProgram, len(episodes))
	copy(out, episodes)
	for i := range out {
		evidence := episodeevidence.Sanitize(out[i].CommunityRating, out[i].Overview, out[i].Tags)
		out[i].CommunityRating = evidence.CommunityRating
		out[i].Overview = evidence.Overview
		out[i].Tags = evidence.Tags
	}
	return out
}

func decodeSeriesEpisodes(blob []byte) ([]schedule.ResolvedProgram, error) {
	if bytes.Equal(bytes.TrimSpace(blob), []byte("null")) {
		return nil, errors.New("episode cache must be a JSON array, not null")
	}
	var encoded []json.RawMessage
	if err := json.Unmarshal(blob, &encoded); err != nil {
		return nil, err
	}
	out := make([]schedule.ResolvedProgram, 0, len(encoded))
	for i, raw := range encoded {
		structural, evidence, err := episodeevidence.DecodeObject(raw)
		if err != nil {
			return nil, fmt.Errorf("episode %d: %w", i, err)
		}
		fields, err := decodeEpisodeObjectFields(structural)
		if err != nil {
			return nil, fmt.Errorf("episode %d: %w", i, err)
		}
		structural, err = validateEpisodeStructuralFields(fields)
		if err != nil {
			return nil, fmt.Errorf("episode %d: %w", i, err)
		}
		var episode schedule.ResolvedProgram
		if err := json.Unmarshal(structural, &episode); err != nil {
			return nil, fmt.Errorf("episode %d structural fields: %w", i, err)
		}
		if err := validateCachedEpisode(episode); err != nil {
			return nil, fmt.Errorf("episode %d: %w", i, err)
		}
		episode.CommunityRating = evidence.CommunityRating
		episode.Overview = evidence.Overview
		episode.Tags = evidence.Tags
		out = append(out, episode)
	}
	return out, nil
}

type episodeObjectField struct {
	name  string
	value json.RawMessage
}

func decodeEpisodeObjectFields(raw json.RawMessage) ([]episodeObjectField, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	opening, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := opening.(json.Delim)
	if !ok || delim != '{' {
		return nil, errors.New("episode must be a JSON object")
	}
	var fields []episodeObjectField
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("episode object member name is not a string")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		fields = append(fields, episodeObjectField{name: name, value: value})
	}
	closing, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return nil, errors.New("episode JSON object is not closed")
	}
	return fields, nil
}

func validateEpisodeStructuralFields(fields []episodeObjectField) ([]byte, error) {
	structuralCounts := make(map[string]int, 5)
	var structural bytes.Buffer
	structural.WriteByte('{')
	first := true
	for _, field := range fields {
		if canonical, required := requiredEpisodeStructuralField(field.name); required {
			structuralCounts[canonical]++
			if structuralCounts[canonical] > 1 {
				return nil, fmt.Errorf("duplicate structural member %s", canonical)
			}
			if err := validateEpisodeStructuralJSONType(canonical, field.value); err != nil {
				return nil, err
			}
		}
		name, err := json.Marshal(field.name)
		if err != nil {
			return nil, err
		}
		if !first {
			structural.WriteByte(',')
		}
		first = false
		structural.Write(name)
		structural.WriteByte(':')
		structural.Write(field.value)
	}
	for _, canonical := range requiredEpisodeStructuralFields {
		if structuralCounts[canonical] == 0 {
			return nil, fmt.Errorf("missing structural member %s", canonical)
		}
	}
	structural.WriteByte('}')
	return structural.Bytes(), nil
}

var requiredEpisodeStructuralFields = [...]string{"LibraryItemID", "DurationMs", "Season", "Episode", "EpisodeEnd"}

func validateEpisodeStructuralJSONType(name string, value json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.UseNumber()
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("structural member %s: %w", name, err)
	}
	if name == "LibraryItemID" {
		if _, ok := token.(string); !ok {
			return fmt.Errorf("structural member %s must be a JSON string", name)
		}
		return nil
	}
	if _, ok := token.(json.Number); !ok {
		return fmt.Errorf("structural member %s must be a JSON number", name)
	}
	return nil
}

func requiredEpisodeStructuralField(name string) (string, bool) {
	for _, canonical := range requiredEpisodeStructuralFields {
		if strings.EqualFold(name, canonical) {
			return canonical, true
		}
	}
	return "", false
}

func validateCachedEpisode(episode schedule.ResolvedProgram) error {
	return episodeevidence.ValidatePlayable(
		episode.LibraryItemID, episode.DurationMs, episode.Season, episode.Episode, episode.EpisodeEnd,
	)
}
