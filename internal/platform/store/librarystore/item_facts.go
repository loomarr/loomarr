package librarystore

import (
	"context"
	"fmt"
	"strings"
)

// LibraryItemFacts is what a guide and a lineup read about a library item (#2151 step 15): the
// facts a media server's item metadata would give, read from the Library's own rows.
type LibraryItemFacts struct {
	Overview string
	Genres   []string
	Year     int
	// Rating is the item's own content rating, else its series'.
	Rating string
	// RuntimeMs is the playing file's own duration (the default edition's, when a movie plays one
	// in each), else the catalog's runtime.
	RuntimeMs int64
}

// LibraryItemFactsReader reads library items' facts.
type LibraryItemFactsReader interface {
	// LibraryItemFacts answers each id that is a library item; any other id is absent. An episode
	// with no genres of its own takes its series'.
	LibraryItemFacts(ctx context.Context, ids []string) (map[string]LibraryItemFacts, error)
}

// libraryFactsChunk bounds one IN list, under SQLite's variable limit.
const libraryFactsChunk = 500

func (s *Store) LibraryItemFacts(ctx context.Context, ids []string) (map[string]LibraryItemFacts, error) {
	out := make(map[string]LibraryItemFacts, len(ids))
	for start := 0; start < len(ids); start += libraryFactsChunk {
		chunk := ids[start:min(start+libraryFactsChunk, len(ids))]
		if err := s.libraryItemFacts(ctx, chunk, out); err != nil {
			return nil, fmt.Errorf("library item facts: %w", err)
		}
	}
	return out, nil
}

func (s *Store) libraryItemFacts(ctx context.Context, ids []string, out map[string]LibraryItemFacts) error {
	in := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT i.id, COALESCE(i.series_id, ''), i.overview, i.year,
		COALESCE(NULLIF(i.rating, ''), sr.rating, ''),
		COALESCE(NULLIF((SELECT src.duration_ms FROM library_sources src WHERE src.item_id = i.id AND src.plays
			ORDER BY src.edition <> '', src.part, src.path LIMIT 1), 0), i.runtime_ms)
		FROM library_items i LEFT JOIN library_items sr ON sr.id = i.series_id
		WHERE i.id IN (`+in+`)`), args...)
	if err != nil {
		return err
	}
	series := map[string]string{}
	for rows.Next() {
		var id, seriesID string
		var f LibraryItemFacts
		if err := rows.Scan(&id, &seriesID, &f.Overview, &f.Year, &f.Rating, &f.RuntimeMs); err != nil {
			_ = rows.Close()
			return err
		}
		out[id] = f
		if seriesID != "" {
			series[id] = seriesID
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	genreIDs := append([]any{}, args...)
	for _, sid := range series {
		genreIDs = append(genreIDs, sid)
	}
	gin := strings.TrimSuffix(strings.Repeat("?, ", len(genreIDs)), ", ")
	grows, err := s.h.QueryContext(ctx, s.ph(`SELECT item_id, name FROM library_genres WHERE item_id IN (`+gin+`)
		ORDER BY item_id, position`), genreIDs...)
	if err != nil {
		return err
	}
	defer func() { _ = grows.Close() }()
	genres := map[string][]string{}
	for grows.Next() {
		var id, name string
		if err := grows.Scan(&id, &name); err != nil {
			return err
		}
		genres[id] = append(genres[id], name)
	}
	if err := grows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		f, ok := out[id]
		if !ok {
			continue
		}
		f.Genres = genres[id]
		if len(f.Genres) == 0 {
			f.Genres = genres[series[id]]
		}
		out[id] = f
	}
	return nil
}
