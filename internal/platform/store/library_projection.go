package store

import (
	"context"
	"database/sql"
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

// saveLibraryItem writes one library_items row whole.
func (s *sqlStore) saveLibraryItem(ctx context.Context, tx *sql.Tx, it LibraryItem, at time.Time) error {
	if it.ID == "" || it.Kind == "" {
		return fmt.Errorf("save library item: id and kind are required")
	}
	sortName := it.SortName
	if sortName == "" {
		sortName = strings.ToLower(it.Name)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO library_items (id, kind, name, sort_name, year, catalog,
		origin_authority, origin_key, grounded, match_basis, match_app, match_page, set_key, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, name = excluded.name,
		sort_name = excluded.sort_name, year = excluded.year, catalog = excluded.catalog,
		origin_authority = excluded.origin_authority, origin_key = excluded.origin_key,
		grounded = excluded.grounded, match_basis = excluded.match_basis, match_app = excluded.match_app,
		match_page = excluded.match_page, set_key = excluded.set_key, updated_at_ms = excluded.updated_at_ms`),
		it.ID, it.Kind, it.Name, sortName, it.Year, it.Catalog.String(), it.Origin.Authority, it.Origin.Key,
		it.Match.Grounded(), it.Match.Basis, it.Match.App, it.Match.Page, it.Set.String(), at.UnixMilli()); err != nil {
		return fmt.Errorf("save library item: %w", err)
	}
	return nil
}
