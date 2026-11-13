package librarystore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// LibraryItem is a movie, series or episode the scanner wrote (#2151 step 10, decisions 40–45).
// Library → Items lists movies and series.
type LibraryItem struct {
	// ID is the inventory item id; Kind is movie, series or episode.
	ID   string
	Kind string
	// Name, SortName and Year are the resolved title, the key it sorts by and its year (0 unknown).
	Name     string
	SortName string
	Year     int
	// Catalog is the library_catalog row its details come from; zero for none.
	Catalog LibraryCatalogKey
	// Set is the library_catalog row of the ready-made set (a TMDB collection) a movie belongs to;
	// zero for none.
	Set LibraryCatalogKey
	// Origin is its scan origin: a movie's file, a series' folder. The maintainer's answer for a
	// title nothing grounds is kept against it.
	Origin LibraryOrigin
	// Match is what says which title it is; the zero value when nothing does, which puts it in
	// Needs you.
	Match LibraryMatch

	// The resolved facts (decision 40), each with who supplied it in From.
	OriginalTitle string
	Overview      string
	// PremiereDate is as the source gave it, such as "2013-05-01"; "" unknown.
	PremiereDate string
	// Rating is the content rating, such as "TV-PG". An episode with none takes its show's, and
	// From.Rating says "show" (decision 42).
	Rating string
	// Score is the community score, 0–10; 0 unknown.
	Score     float64
	RuntimeMs int64
	// Season, Episode and EpisodeEnd number an episode (EpisodeEnd for a multi-episode file).
	Season, Episode, EpisodeEnd int
	// Unnumbered is an episode the scanner could not number by its show's catalog and nobody has
	// said which order its show's files follow: it keeps its file's name and number (decision 53).
	Unnumbered bool
	// Series is an episode's series by its scan origin; the store fills SeriesID from it once the
	// series is written.
	Series   LibraryOrigin
	SeriesID string
	From     LibraryFrom
	// IDs are the item's external ids; Genres, Studios and Tags in the source's order. An episode
	// has no genres or studios of its own: they are its show's (decision 42).
	IDs       []LibraryID
	Genres    []string
	Studios   []string
	Countries []string
	Tags      []string
	// Tagline is the one-line pitch; Trailer a web page that plays the trailer.
	Tagline string
	Trailer string
	// Credits are its cast in billing order, then its key crew (decision 49). An episode has
	// none of its own: its show's apply (decision 42).
	Credits []LibraryCredit
	// Sources are its present files (decision 41), one marked Plays; Art its artwork references.
	Sources []LibrarySource
	Art     []LibraryArt
}

// LibrarySource is one file of a title, with its tracks.
type LibrarySource struct {
	Path      string
	Container string
	SizeBytes int64
	Bitrate   int64
	// DurationMs is the header's length; 0 until it is read.
	DurationMs int64
	// Edition labels a version of the title ("" the default; decision 54).
	Edition string
	// Part is which part of a movie in parts it is, from 1; 0 for a whole file. Every part of the
	// edition that plays is marked Plays, and they play in part order.
	Part int
	// Plays marks the copy that plays (decision 37), or each of its parts.
	Plays   bool
	Streams []LibraryStream
	// Trickplay are the trickplay files beside it, referenced and never imported (decision 61).
	Trickplay []LibraryTrickplay
	// Chapters are its chapter marks, in order; Markers its .edl markers.
	Chapters []LibraryChapter
	Markers  []LibraryMarker
}

// LibraryCredit is one person credited on a title. Role is what they did (actor, director, writer,
// creator); Character an actor's part. A person with a catalog id is one library_people row
// wherever they are credited; one without is kept by name, unlinked (decision 49).
type LibraryCredit struct {
	Name, Role, Character string
	// ID is the person's id in a catalog of people; zero when unlinked.
	ID LibraryID
	// Photo is an http(s) URL of their headshot.
	Photo string
}

// PersonID is the library_people row the credit names: "<namespace>:<id>" when linked, such as
// "tmdb:<id>", else "name:<name>".
func (c LibraryCredit) PersonID() string {
	if id := c.Linked(); id != (LibraryID{}) {
		return id.Namespace + ":" + id.Value
	}
	return "name:" + c.Name
}

// Linked is the person's catalog id when it names both a catalog and an id; zero otherwise.
func (c LibraryCredit) Linked() LibraryID {
	if c.ID.Namespace == "" || c.ID.Value == "" {
		return LibraryID{}
	}
	return c.ID
}

// LibraryChapter is one chapter mark of a source. EndMs is -1 when the container does not say.
type LibraryChapter struct {
	StartMs, EndMs int64
	Title          string
}

// LibraryMarker is one span an .edl file beside a source marks: Kind is "cut", "mute", "scene" or
// "commercial", as the line's action says.
type LibraryMarker struct {
	StartMs, EndMs int64
	Kind           string
}

// LibraryTrickplay is one trickplay file of a source: where it is and its format ("bif").
type LibraryTrickplay struct {
	Ref    string
	Format string
}

// LibraryStream is one track of a file: raw values, and labels computed once (decision 44).
type LibraryStream struct {
	Index                 int
	Kind                  string
	Codec, Profile, Title string
	Default, Forced       bool
	// Language is the ISO 639-1 code where one exists (else as the file says it); LanguageName
	// its English name.
	Language, LanguageName string
	Channels               int
	ChannelLayout          string
	SampleRate             int
	// AudioFormat names an audio track's format, such as "Dolby Digital Plus".
	AudioFormat string
	Width       int
	Height      int
	FrameRate   string
	PixelFormat string
	// ColorTransfer is raw; Resolution ("1080p", "4K") and HDR ("HDR10", "HLG", "") are labels.
	ColorTransfer string
	Resolution    string
	HDR           string
	// Commentary, HearingImpaired (SDH) and Descriptive (it describes the picture) flag a track.
	Commentary, HearingImpaired, Descriptive bool
	// ExternalRef is the file an external track lives in, beside the media file; "" for a track
	// inside it. An external track's Index follows the file's own tracks.
	ExternalRef string
}

// LibraryArt is one artwork reference: a poster, fanart or logo; Season numbers a season poster.
type LibraryArt struct {
	Kind   string
	Season int
	Ref    string
}

// LibraryFrom names the provider each resolved fact came from ("" unknown; RatingFromShow).
type LibraryFrom struct {
	Name, Year, Overview, Premiere, Rating, Score, Runtime string
}

// RatingFromShow is LibraryFrom.Rating for an episode whose rating is its show's.
const RatingFromShow = "show"

// LibraryID is one external id of an item.
type LibraryID struct {
	Namespace, Value string
}

// LibraryMatch is what said which title an item is (#2151 decision 26): "Wrong title?" sends the
// maintainer to the app that decides, or lets them pick when nothing else does.
type LibraryMatch struct {
	// Basis is the evidence, a metadataprovider basis: manager (an arr), embedded_id (an id in a
	// name), sidecar (an NFO) or link (the maintainer's answer). "" is nothing.
	Basis string
	// App is the provider that said it; "" for evidence Loomarr finds itself.
	App string
	// Page is the title's page in that app, for a person to open; "" when it has none.
	Page string
}

// Grounded is true when anything gives the item an id.
func (m LibraryMatch) Grounded() bool { return m.Basis != "" }

// LibraryOrigin is an inventory origin by its authority and key.
type LibraryOrigin struct {
	Authority string
	Key       string
}

// LibraryItemImageOwner is the image owner kind of a library item's artwork (library_item_images). The ref is the
// one record of which image is its poster: an adopted image changes hash when its bytes arrive,
// and the image service moves the refs with it.
const LibraryItemImageOwner = "library_item"

// LibraryItemSummary is an item with what its present files say about it.
type LibraryItemSummary struct {
	LibraryItem
	// Poster, Backdrop and Logo are the image hashes its poster, backdrop (fanart) and clear logo
	// refs name; "" for none.
	Poster, Backdrop, Logo string
	// Files counts its present, importable files (a series' episodes); Pending, those whose header is
	// not read yet. An item with Pending > 0 is still getting ready.
	Files   int
	Pending int
	// Unplayable counts its present files whose header could not be read: they can't be played.
	Unplayable int
	// ExtraCopies counts a movie's extra copies, its files that are not a copy that plays (each
	// edition's, or each part of a movie in parts), and ExtraBytes is what they take.
	ExtraCopies int
	ExtraBytes  int64
}

// LibraryItemQuery narrows the list. Kind "" is movies and series; Search matches the name, any
// case. Problems keeps only the titles that need a look: a file that can't be played, or a movie
// with an extra copy. Limit 0 is every item.
type LibraryItemQuery struct {
	Kind     string
	Search   string
	Problems bool
	// IDs keeps only these items, when not empty.
	IDs []string
	// InSet keeps only the movies in a ready-made set.
	InSet bool
	// Ready keeps only the titles with every file's details read.
	Ready bool
	// Recent lists the titles written last first, instead of by name.
	Recent bool
	Limit  int
	Offset int
}

// LibraryItemChannel is a channel whose lineup names a title.
type LibraryItemChannel struct {
	ID     string
	Name   string
	Number int
}

// LibraryItemPage is one page of items in sort order, and how many match in all.
type LibraryItemPage struct {
	Items []LibraryItemSummary
	Total int
}

// LibraryItemStore keeps the Library's list of movies and series.
type LibraryItemStore interface {
	// SaveLibraryItems writes items whole, outside any inventory write: for fixtures. The scanner's
	// rows come from the LibraryProjection, in step with the inventory.
	SaveLibraryItems(ctx context.Context, items []LibraryItem, at time.Time) error
	// ListLibraryItems lists items with at least one present file, by sort name.
	ListLibraryItems(ctx context.Context, q LibraryItemQuery) (LibraryItemPage, error)
	// LibraryItem reads one item with a present file; ErrNotFound otherwise.
	LibraryItem(ctx context.Context, id string) (LibraryItemSummary, error)
	// LibraryItemChannels lists the channels whose lineup names the item, by number.
	LibraryItemChannels(ctx context.Context, id string) ([]LibraryItemChannel, error)
	// LibraryItemFiles lists an item's present files (a series' episodes), in path order.
	LibraryItemFiles(ctx context.Context, id string) ([]LibraryFile, error)
	// LibraryItemsFiles is LibraryItemFiles for many items at once, by item id: one read for a
	// list, not one per title. An item with no present file has no entry.
	LibraryItemsFiles(ctx context.Context, ids []string) (map[string][]LibraryFile, error)
	// RecheckLibraryItemFiles puts the header of an item's present, importable files (a series'
	// episodes) back to pending, so the scanner reads them again, and lists them.
	RecheckLibraryItemFiles(ctx context.Context, id string) ([]LibraryFilePath, error)
	// RemoveLibraryFile marks a present file missing at `at`, unheld: one Loomarr moved away
	// itself, which no removal guard needs to confirm. A file not present is left alone.
	RemoveLibraryFile(ctx context.Context, path string, at time.Time) error
	// RefreshLibrary is "Refresh everything": every present file is written again on the next
	// pass, and every fetched catalog record is due again (DueLibraryCatalog). Headers and records
	// stay readable meanwhile, so no title stops playing.
	RefreshLibrary(ctx context.Context) error
}

func (s *Store) RefreshLibrary(ctx context.Context) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("refresh library: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE library_files SET identity = '' WHERE missing_at_ms = 0`); err != nil {
		return fmt.Errorf("refresh library files: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE library_catalog SET version = %d WHERE fetched_at_ms > 0`,
		catalogRefresh)); err != nil {
		return fmt.Errorf("refresh library catalog: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("refresh library: %w", err)
	}
	return nil
}

func (s *Store) RecheckLibraryItemFiles(ctx context.Context, id string) ([]LibraryFilePath, error) {
	return s.resetFiles(ctx, `header = '`+LibraryHeaderPending+`'`,
		`(inventory_item_id = ? OR inventory_series_id = ?) AND problem = ''`, id, id)
}

func (s *Store) RemoveLibraryFile(ctx context.Context, path string, at time.Time) error {
	_, err := s.resetFiles(ctx, fmt.Sprintf("missing_at_ms = %d, held = FALSE", at.UnixMilli()), "path = ?", path)
	return err
}

func (s *Store) SaveLibraryItems(ctx context.Context, items []LibraryItem, at time.Time) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("save library items: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, it := range items {
		if err := SaveItemIn(ctx, s, tx, it, at); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library items: %w", err)
	}
	return nil
}

// The files that count toward a title i are its present, importable files: its own, and a series'
// episodes'. Each is read per title through the inventory_item_id and inventory_series_id indexes,
// never by aggregating every file: a page needs its own titles' counts only.

// presentFile is the condition on a library_files row f that it is present and importable.
const presentFile = `f.missing_at_ms = 0 AND f.problem = ''`

// countFiles is how many of title i's files hold also, a condition on f ("" for all).
// itemImage is the image hash a title's ref in role names, or "".
func itemImage(role string) string {
	return `COALESCE((SELECT MIN(r.image_hash) FROM library_item_images r
			WHERE r.item_id = i.id AND r.role = '` + role + `'), '')`
}

func countFiles(also string) string {
	return `((SELECT COUNT(*) FROM library_files f WHERE f.inventory_item_id = i.id AND ` + presentFile + also + `) +
		(SELECT COUNT(*) FROM library_files f WHERE f.inventory_series_id = i.id AND ` + presentFile + also + `))`
}

var (
	// itemListed keeps a title with a file that counts toward it: only those are listed.
	itemListed = `(EXISTS (SELECT 1 FROM library_files f WHERE f.inventory_item_id = i.id AND ` + presentFile + `)
		OR EXISTS (SELECT 1 FROM library_files f WHERE f.inventory_series_id = i.id AND ` + presentFile + `))`
	// itemColumns is a title, its poster, backdrop and logo refs, and its file counts.
	itemColumns = `i.id, i.kind, i.name, i.sort_name, i.year, i.catalog, i.match_basis, i.match_app, i.match_page, i.set_key,
		` + itemImage("poster") + `, ` + itemImage("backdrop") + `, ` + itemImage("logo") + `,
		` + countFiles("") + `, ` + countFiles(` AND f.header = 'pending'`) + `, ` + countFiles(` AND f.header = 'failed'`) + `,
		(SELECT COUNT(*) FROM ` + extraCopies + `), COALESCE((SELECT SUM(s.size_bytes) FROM ` + extraCopies + `), 0)`
	// itemProblems keeps a title that needs a look: a file that cannot play, or a movie's extra copy.
	itemProblems = `(` + countFiles(` AND f.header = 'failed'`) + ` > 0
		OR (i.kind = 'movie' AND EXISTS (SELECT 1 FROM ` + extraCopies + `)))`
)

// extraCopies is title i's files that no edition plays (decision 37), from its rows.
const extraCopies = `library_sources s WHERE s.item_id = i.id AND NOT s.plays`

func (s *Store) ListLibraryItems(ctx context.Context, q LibraryItemQuery) (LibraryItemPage, error) {
	where := []string{}
	args := []any{}
	switch q.Kind {
	case "":
		where = append(where, "i.kind IN ('movie', 'series')")
	default:
		where = append(where, "i.kind = ?")
		args = append(args, q.Kind)
	}
	if search := strings.TrimSpace(q.Search); search != "" {
		where = append(where, s.lower("i.name")+" LIKE ? ESCAPE '\\'")
		args = append(args, "%"+likeEscape(strings.ToLower(search))+"%")
	}
	if q.InSet {
		where = append(where, "i.set_key <> ''")
	}
	if len(q.IDs) > 0 {
		where = append(where, "i.id IN ("+strings.TrimSuffix(strings.Repeat("?, ", len(q.IDs)), ", ")+")")
		for _, id := range q.IDs {
			args = append(args, id)
		}
	}
	// A title the maintainer said is not one is left out.
	where = append(where, `NOT EXISTS (SELECT 1 FROM library_answers a WHERE a.origin_authority = i.origin_authority
		AND a.origin_key = i.origin_key AND a.ignored = TRUE)`)
	where = append(where, itemListed)
	if q.Problems {
		where = append(where, itemProblems)
	}
	if q.Ready {
		where = append(where, countFiles(` AND f.header = 'pending'`)+` = 0`)
	}
	from := ` FROM library_items i WHERE ` + strings.Join(where, " AND ")
	var page LibraryItemPage
	order := ` ORDER BY i.sort_name, i.id`
	if q.Recent {
		order = ` ORDER BY i.updated_at_ms DESC, i.id`
	}
	query := `SELECT ` + itemColumns + from + order
	if q.Limit > 0 {
		// A page counts the whole list on its own; every item is its own count.
		if err := s.h.QueryRowContext(ctx, s.ph(`SELECT COUNT(*)`+from), args...).Scan(&page.Total); err != nil {
			return page, fmt.Errorf("count library items: %w", err)
		}
		query += ` LIMIT ? OFFSET ?`
		args = append(args, q.Limit, q.Offset)
	}
	items, err := s.scanLibraryItems(ctx, query, args...)
	if err != nil {
		return page, err
	}
	page.Items = items
	if q.Limit <= 0 {
		page.Total = len(items)
	}
	return page, nil
}

func (s *Store) LibraryItemChannels(ctx context.Context, id string) ([]LibraryItemChannel, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT c.id, c.name, c.number FROM channels c
		WHERE EXISTS (SELECT 1 FROM channel_lineup l JOIN titles t ON t.key = l.title_key
			WHERE l.channel_id = c.id AND t.library_id = ?)
		ORDER BY c.number, c.id`), id)
	if err != nil {
		return nil, fmt.Errorf("library item channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []LibraryItemChannel{}
	for rows.Next() {
		var c LibraryItemChannel
		if err := rows.Scan(&c.ID, &c.Name, &c.Number); err != nil {
			return nil, fmt.Errorf("library item channels: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) LibraryItem(ctx context.Context, id string) (LibraryItemSummary, error) {
	items, err := s.scanLibraryItems(ctx, `SELECT `+itemColumns+`
		FROM library_items i WHERE i.id = ? AND `+itemListed, id)
	if err != nil {
		return LibraryItemSummary{}, err
	}
	if len(items) == 0 {
		return LibraryItemSummary{}, storedb.ErrNotFound
	}
	return items[0], nil
}

func (s *Store) scanLibraryItems(ctx context.Context, query string, args ...any) ([]LibraryItemSummary, error) {
	rows, err := s.h.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list library items: %w", err)
	}
	out := []LibraryItemSummary{}
	for rows.Next() {
		var it LibraryItemSummary
		var catalog, set string
		if err := rows.Scan(&it.ID, &it.Kind, &it.Name, &it.SortName, &it.Year, &catalog, &it.Match.Basis,
			&it.Match.App, &it.Match.Page, &set, &it.Poster, &it.Backdrop, &it.Logo, &it.Files, &it.Pending, &it.Unplayable, &it.ExtraCopies, &it.ExtraBytes); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library items: %w", err)
		}
		it.Catalog, _ = ParseLibraryCatalogKey(catalog)
		it.Set, _ = ParseLibraryCatalogKey(set)
		out = append(out, it)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library items: %w", err)
	}
	return out, nil
}

func (s *Store) LibraryItemFiles(ctx context.Context, id string) ([]LibraryFile, error) {
	files, err := s.LibraryItemsFiles(ctx, []string{id})
	return files[id], err
}

// idsPerRead keeps one read's id list well inside either engine's bound on a query's parameters.
const idsPerRead = 500

func (s *Store) LibraryItemsFiles(ctx context.Context, ids []string) (map[string][]LibraryFile, error) {
	out := map[string][]LibraryFile{}
	for len(ids) > 0 {
		batch := ids[:min(len(ids), idsPerRead)]
		ids = ids[len(batch):]
		if err := s.readItemsFiles(ctx, batch, out); err != nil {
			return nil, fmt.Errorf("list library item files: %w", err)
		}
	}
	return out, nil
}

// readItemsFiles adds the present files of a batch of items to out. A file counts toward its own
// item and its series, whichever of them the batch names.
func (s *Store) readItemsFiles(ctx context.Context, ids []string, out map[string][]LibraryFile) error {
	in := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
	args := make([]any, 0, 2*len(ids))
	named := make(map[string]bool, len(ids))
	for _, id := range ids {
		args, named[id] = append(args, id), true
	}
	args = append(args, args...)
	rows, err := s.h.QueryContext(ctx, s.ph(`SELECT path, root, size_bytes, modified_ns, identity, kind, header,
		problem, held, seen_at_ms, missing_at_ms, COALESCE(inventory_item_id, ''), COALESCE(inventory_series_id, '')
		FROM library_files WHERE (inventory_item_id IN (`+in+`) OR inventory_series_id IN (`+in+`))
		AND missing_at_ms = 0 AND problem = '' ORDER BY path`), args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var f LibraryFile
		var seen, missing int64
		if err := rows.Scan(&f.Path, &f.Root, &f.Size, &f.ModifiedNs, &f.Identity, &f.Kind, &f.Header, &f.Problem,
			&f.Held, &seen, &missing, &f.ItemID, &f.SeriesID); err != nil {
			_ = rows.Close()
			return err
		}
		f.SeenAt = time.UnixMilli(seen).UTC()
		for _, id := range []string{f.ItemID, f.SeriesID} {
			if id != "" && named[id] {
				out[id] = append(out[id], f)
			}
		}
	}
	return errors.Join(rows.Err(), rows.Close())
}

// likeEscape escapes LIKE's wildcards so a search matches them literally.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
