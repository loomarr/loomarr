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
}

// LibraryItemQuery narrows the list. Kind "" is movies and series; Search matches the name, any
// case. Limit 0 is every item.
type LibraryItemQuery struct {
	Kind   string
	Search string
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
	// SaveLibraryItems writes items whole.
	SaveLibraryItems(ctx context.Context, items []LibraryItem, at time.Time) error
	// ListLibraryItems lists items with at least one present file, by sort name.
	ListLibraryItems(ctx context.Context, q LibraryItemQuery) (LibraryItemPage, error)
	// LibraryItem reads one item with a present file; ErrNotFound otherwise.
	LibraryItem(ctx context.Context, id string) (LibraryItemSummary, error)
	// LibraryItemFiles lists an item's present files (a series' episodes), in path order.
	LibraryItemFiles(ctx context.Context, id string) ([]LibraryFile, error)
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
	stmt, err := tx.PrepareContext(ctx, s.ph(`INSERT INTO library_items (id, kind, name, sort_name, year, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, name = excluded.name,
		sort_name = excluded.sort_name, year = excluded.year, updated_at = excluded.updated_at`))
	if err != nil {
		return fmt.Errorf("save library items: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, it := range items {
		if it.ID == "" || it.Kind == "" {
			return fmt.Errorf("save library items: id and kind are required")
		}
		sortName := it.SortName
		if sortName == "" {
			sortName = strings.ToLower(it.Name)
		}
		if _, err := stmt.ExecContext(ctx, it.ID, it.Kind, it.Name, sortName, it.Year, at.UnixMilli()); err != nil {
			return fmt.Errorf("save library item: %w", err)
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
	SELECT item_id AS id, header FROM library_files WHERE item_id <> '' AND missing_at = 0 AND problem = ''
	UNION ALL
	SELECT series_id AS id, header FROM library_files WHERE series_id <> '' AND missing_at = 0 AND problem = ''
)`

// itemColumns is an item, its poster ref and its file counts, read over presentFiles; itemGroup
// groups by the item.
const (
	itemColumns = `i.id, i.kind, i.name, i.sort_name, i.year,
		COALESCE((SELECT MIN(r.image_hash) FROM image_refs r WHERE r.owner_kind = '` + LibraryItemImageOwner + `'
			AND r.owner_id = i.id AND r.role = 'poster'), ''),
		COUNT(*), COALESCE(SUM(CASE WHEN f.header = 'pending' THEN 1 ELSE 0 END), 0)`
	itemGroup = `i.id, i.kind, i.name, i.sort_name, i.year`
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
	from := ` FROM library_items i JOIN ` + presentFiles + ` f ON f.id = i.id WHERE ` + strings.Join(where, " AND ")

	var page LibraryItemPage
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(DISTINCT i.id)`+from), args...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count library items: %w", err)
	}
	query := `SELECT ` + itemColumns + from + ` GROUP BY ` + itemGroup + ` ORDER BY i.sort_name, i.id`
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
		if err := rows.Scan(&it.ID, &it.Kind, &it.Name, &it.SortName, &it.Year, &it.Poster, &it.Files, &it.Pending); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list library items: %w", err)
		}
		out = append(out, it)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("list library items: %w", err)
	}
	return out, nil
}

func (s *sqlStore) LibraryItemFiles(ctx context.Context, id string) ([]LibraryFile, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT path, root, size, modified_ns, identity, kind, header,
		problem, held, seen_at, missing_at, item_id, series_id FROM library_files
		WHERE (item_id = ? OR series_id = ?) AND missing_at = 0 AND problem = '' ORDER BY path`), id, id)
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
