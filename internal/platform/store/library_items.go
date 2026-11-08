package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LibraryItem is a movie or series the scanner wrote, as Library → Items lists it (#2151 step 10).
type LibraryItem struct {
	// ID is the inventory item id; Kind is movie or series.
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

// LibraryItemImageOwner is the image_refs owner kind of a library item's artwork. The ref is the
// one record of which image is its poster: an adopted image changes hash when its bytes arrive,
// and the image service moves the refs with it.
const LibraryItemImageOwner = "library_item"

// LibraryItemSummary is an item with what its present files say about it.
type LibraryItemSummary struct {
	LibraryItem
	// Poster is the image hash its poster ref names; "" for none.
	Poster string
	// Files counts its present, importable files (a series' episodes); Pending, those whose header is
	// not read yet. An item with Pending > 0 is still getting ready.
	Files   int
	Pending int
	// Unplayable counts its present files whose header could not be read: they can't be played.
	Unplayable int
}

// LibraryItemQuery narrows the list. Kind "" is movies and series; Search matches the name, any
// case. Problems keeps only the titles that need a look: a file that can't be played, or a movie
// with more than one file (all but one are extra copies). Limit 0 is every item.
type LibraryItemQuery struct {
	Kind     string
	Search   string
	Problems bool
	// IDs keeps only these items, when not empty.
	IDs []string
	// InSet keeps only the movies in a ready-made set.
	InSet  bool
	Limit  int
	Offset int
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
	// LibraryItemFiles lists an item's present files (a series' episodes), in path order.
	LibraryItemFiles(ctx context.Context, id string) ([]LibraryFile, error)
	// RecheckLibraryItemFiles puts the header of an item's present, importable files (a series'
	// episodes) back to pending, so the scanner reads them again, and lists them.
	RecheckLibraryItemFiles(ctx context.Context, id string) ([]LibraryFilePath, error)
	// RemoveLibraryFile marks a present file missing at `at`, unheld: one Loomarr moved away
	// itself, which no removal guard needs to confirm. A file not present is left alone.
	RemoveLibraryFile(ctx context.Context, path string, at time.Time) error
}

func (s *sqlStore) RecheckLibraryItemFiles(ctx context.Context, id string) ([]LibraryFilePath, error) {
	return s.resetFiles(ctx, `header = '`+LibraryHeaderPending+`'`,
		`(inventory_item_id = ? OR inventory_series_id = ?) AND problem = ''`, id, id)
}

func (s *sqlStore) RemoveLibraryFile(ctx context.Context, path string, at time.Time) error {
	_, err := s.resetFiles(ctx, fmt.Sprintf("missing_at_ms = %d, held = FALSE", at.UnixMilli()), "path = ?", path)
	return err
}

func (s *sqlStore) SaveLibraryItems(ctx context.Context, items []LibraryItem, at time.Time) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save library items: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, it := range items {
		if err := s.saveLibraryItem(ctx, tx, it, at); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save library items: %w", err)
	}
	return nil
}

// presentFiles is each present, importable file under the item it counts toward: its own item, and
// an episode's series too.
const presentFiles = `(
	SELECT inventory_item_id AS id, header FROM library_files
	WHERE inventory_item_id <> '' AND missing_at_ms = 0 AND problem = ''
	UNION ALL
	SELECT inventory_series_id AS id, header FROM library_files
	WHERE inventory_series_id <> '' AND missing_at_ms = 0 AND problem = ''
)`

// itemColumns is an item, its poster ref and its file counts, read over presentFiles; itemGroup
// groups by the item.
const (
	itemColumns = `i.id, i.kind, i.name, i.sort_name, i.year, i.catalog, i.match_basis, i.match_app, i.match_page, i.set_key,
		COALESCE((SELECT MIN(r.image_hash) FROM image_refs r WHERE r.owner_kind = '` + LibraryItemImageOwner + `'
			AND r.owner_id = i.id AND r.role = 'poster'), ''),
		COUNT(*), COALESCE(SUM(CASE WHEN f.header = 'pending' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN f.header = 'failed' THEN 1 ELSE 0 END), 0)`
	itemGroup = `i.id, i.kind, i.name, i.sort_name, i.year, i.catalog, i.match_basis, i.match_app, i.match_page, i.set_key`
	// itemProblems keeps a group that needs a look.
	itemProblems = `SUM(CASE WHEN f.header = 'failed' THEN 1 ELSE 0 END) > 0 OR (i.kind = 'movie' AND COUNT(*) > 1)`
)

func (s *sqlStore) ListLibraryItems(ctx context.Context, q LibraryItemQuery) (LibraryItemPage, error) {
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
		where = append(where, "LOWER(i.name) LIKE ? ESCAPE '\\'")
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
	from := ` FROM library_items i JOIN ` + presentFiles + ` f ON f.id = i.id WHERE ` + strings.Join(where, " AND ")

	group := ` GROUP BY ` + itemGroup
	if q.Problems {
		group += ` HAVING ` + itemProblems
	}
	var page LibraryItemPage
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM (SELECT i.id`+from+group+`) t`), args...).
		Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count library items: %w", err)
	}
	query := `SELECT ` + itemColumns + from + group + ` ORDER BY i.sort_name, i.id`
	if q.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, q.Limit, q.Offset)
	}
	items, err := s.scanLibraryItems(ctx, query, args...)
	if err != nil {
		return page, err
	}
	page.Items = items
	return page, nil
}

func (s *sqlStore) LibraryItem(ctx context.Context, id string) (LibraryItemSummary, error) {
	items, err := s.scanLibraryItems(ctx, `SELECT `+itemColumns+`
		FROM library_items i JOIN `+presentFiles+` f ON f.id = i.id WHERE i.id = ?
		GROUP BY `+itemGroup, id)
	if err != nil {
		return LibraryItemSummary{}, err
	}
	if len(items) == 0 {
		return LibraryItemSummary{}, ErrNotFound
	}
	return items[0], nil
}

func (s *sqlStore) scanLibraryItems(ctx context.Context, query string, args ...any) ([]LibraryItemSummary, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list library items: %w", err)
	}
	out := []LibraryItemSummary{}
	for rows.Next() {
		var it LibraryItemSummary
		var catalog, set string
		if err := rows.Scan(&it.ID, &it.Kind, &it.Name, &it.SortName, &it.Year, &catalog, &it.Match.Basis,
			&it.Match.App, &it.Match.Page, &set, &it.Poster, &it.Files, &it.Pending, &it.Unplayable); err != nil {
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

func (s *sqlStore) LibraryItemFiles(ctx context.Context, id string) ([]LibraryFile, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT path, root, size_bytes, modified_ns, identity, kind, header,
		problem, held, seen_at_ms, missing_at_ms, inventory_item_id, inventory_series_id FROM library_files
		WHERE (inventory_item_id = ? OR inventory_series_id = ?) AND missing_at_ms = 0 AND problem = ''
		ORDER BY path`), id, id)
	if err != nil {
		return nil, fmt.Errorf("list library item files: %w", err)
	}
	var out []LibraryFile
	for rows.Next() {
		var f LibraryFile
		var seen, missing int64
		if err := rows.Scan(&f.Path, &f.Root, &f.Size, &f.ModifiedNs, &f.Identity, &f.Kind, &f.Header, &f.Problem,
			&f.Held, &seen, &missing, &f.ItemID, &f.SeriesID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library item files: %w", err)
		}
		f.SeenAt = time.UnixMilli(seen).UTC()
		out = append(out, f)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library item files: %w", err)
	}
	return out, nil
}

// likeEscape escapes LIKE's wildcards so a search matches them literally.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
