package store

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// The catalog's retention (#2528): what the importers and the scanner keep after a title or file
// has gone. Fixed product policy, like the filler tables', so there is no setting to get wrong.
const (
	// CatalogMissingRetention is how long a report, source or file the importers stopped seeing is
	// kept. A file only goes missing after the walk's mass-disappearance guard let it, and one that
	// comes back inside the window keeps its row.
	CatalogMissingRetention = 180 * 24 * time.Hour
	// LibraryItemRetention is how long a title's library row outlives its last file. A title that
	// comes back inside it finds its row, details and collections again; one that comes back later
	// is written afresh, and a person's identification answer (library_answers, keyed by where the
	// title is) still applies to it.
	LibraryItemRetention = 365 * 24 * time.Hour
)

// CatalogRetentionStore deletes what the catalog keeps after titles and files have gone.
type CatalogRetentionStore interface {
	// PurgeSpentCatalog deletes the catalog rows nothing will read again and returns how many.
	PurgeSpentCatalog(ctx context.Context, now time.Time) (int, error)
}

// PurgeSpentCatalog deletes, in one transaction and children before parents:
//
//   - importer reports and their sources missing for CatalogMissingRetention, then a source or
//     item with no report left (their measurements, ids and analysis go with them);
//   - files a walk stopped finding CatalogMissingRetention ago, unless their removal is still held
//     for a person to confirm;
//   - library rows older than LibraryItemRetention with no item, file, episode or collection
//     membership left and that no stored record names, with their candidates and image refs;
//   - catalog records no library row names, fetched (or failing) longer than LibraryItemRetention.
//
// "Names" is LibraryReferences: titles, airings, channels' slots and open proposals' picks.
func (s *sqlStore) PurgeSpentCatalog(ctx context.Context, now time.Time) (int, error) {
	refs, err := s.LibraryReferences(ctx)
	if err != nil {
		return 0, fmt.Errorf("purge spent catalog: %w", err)
	}
	missing := now.Add(-CatalogMissingRetention)
	itemCutoffMs := now.Add(-LibraryItemRetention).UnixMilli()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("purge spent catalog: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	total := 0
	exec := func(what, query string, args ...any) error {
		res, err := tx.ExecContext(ctx, s.ph(query), args...)
		if err != nil {
			return fmt.Errorf("purge spent %s: %w", what, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("purge spent %s: count: %w", what, err)
		}
		total += int(n)
		return nil
	}
	for _, step := range []struct {
		what, query string
		args        []any
	}{
		{"source reports", `DELETE FROM inventory_source_origins
			WHERE missing_at IS NOT NULL AND missing_at < ?`, []any{epoch(missing)}},
		{"item reports", `DELETE FROM inventory_item_origins
			WHERE missing_at IS NOT NULL AND missing_at < ?`, []any{epoch(missing)}},
		{"sources", `DELETE FROM inventory_sources WHERE updated_at < ?
			AND NOT EXISTS (SELECT 1 FROM inventory_source_origins o WHERE o.source_id = inventory_sources.id)`,
			[]any{epoch(missing)}},
		{"items", `DELETE FROM inventory_items WHERE updated_at < ?
			AND NOT EXISTS (SELECT 1 FROM inventory_item_origins o WHERE o.item_id = inventory_items.id)`,
			[]any{epoch(missing)}},
		{"missing files", `DELETE FROM library_files
			WHERE missing_at_ms > 0 AND missing_at_ms < ? AND NOT held`, []any{missing.UnixMilli()}},
	} {
		if err := exec(step.what, step.query, step.args...); err != nil {
			return 0, err
		}
	}

	candidates, err := s.distinctStrings(ctx, tx, `SELECT id FROM library_items i WHERE i.updated_at_ms < ?
		AND NOT EXISTS (SELECT 1 FROM inventory_items v WHERE v.id = i.id)
		AND NOT EXISTS (SELECT 1 FROM library_files f WHERE f.inventory_item_id = i.id OR f.inventory_series_id = i.id)
		AND NOT EXISTS (SELECT 1 FROM library_items e WHERE e.series_id = i.id)
		AND NOT EXISTS (SELECT 1 FROM library_collection_members m WHERE m.inventory_item_id = i.id)`, itemCutoffMs)
	if err != nil {
		return 0, fmt.Errorf("purge spent library items: %w", err)
	}
	for _, id := range candidates {
		if _, named := slices.BinarySearch(refs.Items, id); named {
			continue
		}
		// Its candidates and artwork refs go with it (library_candidates and library_item_images
		// cascade).
		if err := exec("library items", `DELETE FROM library_items WHERE id = ?`, id); err != nil {
			return 0, err
		}
	}

	// A record not yet fetched is asked for while a scan resolves its title, before that title's
	// row is written, so it goes only once its fetch has been failing for the whole window.
	if err := exec("catalog records", `DELETE FROM library_catalog
		WHERE ((fetched_at_ms > 0 AND fetched_at_ms < ?) OR (fetched_at_ms = 0 AND retry_at_ms > 0 AND retry_at_ms < ?))
		AND NOT EXISTS (SELECT 1 FROM library_items i
		    WHERE i.catalog = library_catalog.provider || ':' || library_catalog.kind || ':' || library_catalog.external_id
		       OR i.set_key = library_catalog.provider || ':' || library_catalog.kind || ':' || library_catalog.external_id)`,
		itemCutoffMs, itemCutoffMs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("purge spent catalog: %w", err)
	}
	return total, nil
}
