package librarystore

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// The wall's filters (#4476, library.md "Read path") and the choices each one offers. Every
// count and every choice reads listedTitles, so it agrees with the list.

// LibraryHDRAny keeps a title with high dynamic range of any kind.
const LibraryHDRAny = "any"

// libraryHDRTypes are the labels a video track's hdr holds, in the order the choices list them.
var libraryHDRTypes = []string{"Dolby Vision", "HDR10", "HLG", "HDR"}

// library4K is the resolution label of a 4K video track.
const library4K = "4K"

// playableFile keeps a file f a title can play from: present, its header not failed.
const playableFile = presentFile + ` AND f.header <> 'failed'`

// playingTrack is the condition on title i that a track st matching cond is in the copy that
// plays of one of its playable files: a movie's own, or any episode's listed under a series.
// The matching titles are gathered once for the whole list, not looked for title by title: a
// correlated form reads every track of the language for each title. cond is written twice, so
// its arguments are returned twice.
func playingTrack(cond string, args ...any) (string, []any) {
	under := func(column string) string {
		return `SELECT f.` + column + ` FROM library_streams st
			JOIN library_sources src ON src.item_id = st.item_id AND src.path = st.path
			JOIN library_files f ON f.path = src.path AND f.inventory_item_id = src.item_id
			WHERE f.` + column + ` IS NOT NULL AND ` + playableFile + ` AND src.plays AND ` + cond
	}
	return "i.id IN (" + under("inventory_item_id") + " UNION ALL " + under("inventory_series_id") + ")",
		slices.Concat(args, args)
}

// TrackFilterQuery is the list's count under a track filter, for the plan test.
func TrackFilterQuery() string {
	cond, _ := playingTrack(`st.kind = 'audio' AND st.language = 'en'`)
	return `SELECT COUNT(*) FROM library_items i WHERE i.kind = 'movie' AND ` + cond
}

// filterTitles is the conditions, and their arguments, q's filters put on a title i. They AND
// together; each track filter may be met by a different file.
func (s *Store) filterTitles(q LibraryItemQuery) ([]string, []any) {
	var where []string
	var args []any
	add := func(value, cond string) {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			where = append(where, cond)
			args = append(args, value)
		}
	}
	add(q.Genre, s.genreIs())
	add(q.Studio, `EXISTS (SELECT 1 FROM library_studios ls WHERE ls.item_id = i.id AND `+s.lower("ls.name")+` = ?)`)
	if q.Person != "" {
		where = append(where, `EXISTS (SELECT 1 FROM library_credits c WHERE c.item_id = i.id AND c.person_id = ?)`)
		args = append(args, q.Person)
	}
	track := func(cond string, values ...any) {
		cond, values = playingTrack(cond, values...)
		where, args = append(where, cond), append(args, values...)
	}
	for _, language := range []struct{ kind, code string }{{"audio", q.Audio}, {"subtitle", q.Subtitles}} {
		if code := strings.ToLower(strings.TrimSpace(language.code)); code != "" {
			track(`st.kind = '`+language.kind+`' AND `+s.lower("st.language")+` = ?`, code)
		}
	}
	if q.UHD {
		track(`st.kind = 'video' AND st.resolution = '` + library4K + `'`)
	}
	switch q.HDR {
	case "":
	case LibraryHDRAny:
		track(`st.kind = 'video' AND st.hdr <> ''`)
	default:
		track(`st.kind = 'video' AND st.hdr = ?`, q.HDR)
	}
	return where, args
}

// LibraryFacet is one choice of a filter and how many listed titles it keeps. Value is what the
// filter takes; Name what to call it.
type LibraryFacet struct {
	Value  string
	Name   string
	Titles int
}

// LibraryFacets are the choices of the studio, language, 4K and HDR filters.
type LibraryFacets struct {
	Studios   []LibraryFacet
	Audio     []LibraryFacet
	Subtitles []LibraryFacet
	// UHD is how many titles the 4K filter keeps; HDRAny how many any HDR keeps.
	UHD    int
	HDRAny int
	HDR    []LibraryFacet
}

// LibraryPerson is someone a listed title credits. Titles counts the listed titles that do.
type LibraryPerson struct {
	ID     string
	Name   string
	Titles int
}

func (s *Store) LibraryFacets(ctx context.Context, kind string, member bool) (LibraryFacets, error) {
	var out LibraryFacets
	var err error
	if out.Studios, err = s.studioFacets(ctx, kind, member); err != nil {
		return out, err
	}
	return out, s.trackFacets(ctx, kind, member, &out)
}

func (s *Store) studioFacets(ctx context.Context, kind string, member bool) ([]LibraryFacet, error) {
	where, args := listedTitles(kind, member)
	name := s.lower("ls.name")
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT MIN(ls.name), COUNT(DISTINCT i.id)
		FROM library_items i JOIN library_studios ls ON ls.item_id = i.id
		WHERE `+strings.Join(where, " AND ")+`
		GROUP BY `+name+` ORDER BY `+name), args...)
	if err != nil {
		return nil, fmt.Errorf("library studios: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []LibraryFacet{}
	for rows.Next() {
		var f LibraryFacet
		if err := rows.Scan(&f.Name, &f.Titles); err != nil {
			return nil, fmt.Errorf("library studios: %w", err)
		}
		f.Value = f.Name
		out = append(out, f)
	}
	return out, rows.Err()
}

// trackFacets fills the choices read from tracks, in one read of the playing tracks of every
// listed title: the same tracks playingTrack matches.
func (s *Store) trackFacets(ctx context.Context, kind string, member bool, out *LibraryFacets) error {
	where, args := listedTitles(kind, member)
	under := func(column string) string {
		return `SELECT l.id AS title, f.inventory_item_id AS item, f.path AS path
			FROM listed l JOIN library_files f ON f.` + column + ` = l.id WHERE ` + playableFile
	}
	rows, err := s.h.QueryContext(ctx, s.ph(`WITH listed AS (SELECT i.id FROM library_items i WHERE `+
		strings.Join(where, " AND ")+`),
		played AS (`+under("inventory_item_id")+` UNION ALL `+under("inventory_series_id")+`)
		SELECT DISTINCT p.title, st.kind, `+s.lower("st.language")+`, st.language_name, st.resolution, st.hdr
		FROM played p
		JOIN library_sources src ON src.item_id = p.item AND src.path = p.path
		JOIN library_streams st ON st.item_id = src.item_id AND st.path = src.path
		WHERE src.plays`), args...)
	if err != nil {
		return fmt.Errorf("library track choices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	audio, subtitles, hdr := facetCount{}, facetCount{}, facetCount{}
	uhd, anyHDR := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var title, track, language, languageName, resolution, dynamic string
		if err := rows.Scan(&title, &track, &language, &languageName, &resolution, &dynamic); err != nil {
			return fmt.Errorf("library track choices: %w", err)
		}
		switch track {
		case "audio":
			audio.add(language, languageName, title)
		case "subtitle":
			subtitles.add(language, languageName, title)
		case "video":
			if resolution == library4K {
				uhd[title] = true
			}
			if dynamic != "" {
				anyHDR[title] = true
				hdr.add(dynamic, dynamic, title)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("library track choices: %w", err)
	}
	out.Audio, out.Subtitles = audio.byName(), subtitles.byName()
	out.UHD, out.HDRAny = len(uhd), len(anyHDR)
	out.HDR = hdr.byName()
	slices.SortStableFunc(out.HDR, func(a, b LibraryFacet) int {
		return cmp.Compare(slices.Index(libraryHDRTypes, a.Value), slices.Index(libraryHDRTypes, b.Value))
	})
	return nil
}

// facetCount gathers, by value, a choice's name and the titles it keeps.
type facetCount map[string]*struct {
	name   string
	titles map[string]bool
}

// add counts title under value; "" is a track that states none. A value takes the first of its
// names in order, and stands for its own name when it has none.
func (c facetCount) add(value, name, title string) {
	if value == "" {
		return
	}
	f := c[value]
	if f == nil {
		f = &struct {
			name   string
			titles map[string]bool
		}{titles: map[string]bool{}}
		c[value] = f
	}
	if name != "" && (f.name == "" || name < f.name) {
		f.name = name
	}
	f.titles[title] = true
}

func (c facetCount) byName() []LibraryFacet {
	out := make([]LibraryFacet, 0, len(c))
	for value, f := range c {
		out = append(out, LibraryFacet{Value: value, Name: cmp.Or(f.name, value), Titles: len(f.titles)})
	}
	slices.SortFunc(out, func(a, b LibraryFacet) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Value, b.Value))
	})
	return out
}

// peopleQuery reads the people the listed titles credit, narrowed by also.
func (s *Store) peopleQuery(kind string, member bool, also string, alsoArg any) (string, []any) {
	where, args := listedTitles(kind, member)
	return `SELECT p.id, p.name, COUNT(DISTINCT i.id)
		FROM library_items i
		JOIN library_credits c ON c.item_id = i.id
		JOIN library_people p ON p.id = c.person_id
		WHERE ` + strings.Join(append(where, also), " AND ") + `
		GROUP BY p.id, p.name`, append(args, alsoArg)
}

// LibraryPeople searches the credited people by part of their name, any case: the most credited
// first, then by name, at most limit. An empty term lists the most credited.
func (s *Store) LibraryPeople(ctx context.Context, term, kind string, member bool, limit int) ([]LibraryPerson, error) {
	pattern := "%" + likeEscape(strings.ToLower(strings.TrimSpace(term))) + "%"
	query, args := s.peopleQuery(kind, member, s.lower("p.name")+` LIKE ? ESCAPE '\'`, pattern)
	rows, err := s.h.QueryContext(ctx, s.ph(query+` ORDER BY COUNT(DISTINCT i.id) DESC, `+
		s.lower("p.name")+`, p.id LIMIT ?`), append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("library people: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []LibraryPerson{}
	for rows.Next() {
		var p LibraryPerson
		if err := rows.Scan(&p.ID, &p.Name, &p.Titles); err != nil {
			return nil, fmt.Errorf("library people: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LibraryPerson reads one credited person; ErrNotFound when no title the caller reads credits
// them, the same as an id that names nobody.
func (s *Store) LibraryPerson(ctx context.Context, id string, member bool) (LibraryPerson, error) {
	query, args := s.peopleQuery("", member, "c.person_id = ?", id)
	var p LibraryPerson
	err := s.h.QueryRowContext(ctx, s.ph(query), args...).Scan(&p.ID, &p.Name, &p.Titles)
	if errors.Is(err, sql.ErrNoRows) {
		return p, storedb.ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("library person: %w", err)
	}
	return p, nil
}
