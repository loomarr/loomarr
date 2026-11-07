-- +goose Up
-- Schema naming (#2323), the library tables (#2319, #2320). Renames only; no value changes.
--
-- A time column's name says its unit: `_at` is unix seconds, `_at_ms` unix milliseconds, which is
-- what every one of these holds. A query that joins them to a seconds column now reads the unit.
--
-- An id column's name says its id space. The library's own titles are inventory items, so
-- `library_files`, `library_candidates` and `library_collection_members` name `inventory_item_id`. `library_item_paths.item_id`
-- is the media server's id and keeps its name until decision 0046 retires it. There is no foreign
-- key to `inventory_items`: re-identifying a title deletes the orphaned inventory item
-- (`internal/store/inventory.go`) before the next scan pass rewrites `library_items`.
--
-- `library_files.size` is in bytes; the indexes take the `idx_<table>_<purpose>` form.
ALTER TABLE library_roots RENAME COLUMN checked_at TO checked_at_ms;
ALTER TABLE library_roots RENAME COLUMN added_at TO added_at_ms;
ALTER TABLE library_roots RENAME COLUMN scanned_at TO scanned_at_ms;
ALTER TABLE library_root_paths RENAME COLUMN seen_at TO seen_at_ms;
ALTER TABLE library_files RENAME COLUMN seen_at TO seen_at_ms;
ALTER TABLE library_files RENAME COLUMN missing_at TO missing_at_ms;
ALTER TABLE library_files RENAME COLUMN size TO size_bytes;
ALTER TABLE library_files RENAME COLUMN item_id TO inventory_item_id;
ALTER TABLE library_files RENAME COLUMN series_id TO inventory_series_id;
ALTER TABLE library_items RENAME COLUMN updated_at TO updated_at_ms;
ALTER TABLE library_catalog RENAME COLUMN fetched_at TO fetched_at_ms;
ALTER TABLE library_catalog RENAME COLUMN retry_at TO retry_at_ms;
ALTER TABLE library_answers RENAME COLUMN answered_at TO answered_at_ms;
ALTER TABLE library_candidates RENAME COLUMN item_id TO inventory_item_id;
ALTER TABLE library_collections RENAME COLUMN created_at TO created_at_ms;
ALTER TABLE library_collections RENAME COLUMN updated_at TO updated_at_ms;
ALTER TABLE library_collection_members RENAME COLUMN item_id TO inventory_item_id;

ALTER INDEX library_files_root RENAME TO idx_library_files_root;
ALTER INDEX library_files_item RENAME TO idx_library_files_inventory_item;
ALTER INDEX library_files_series RENAME TO idx_library_files_inventory_series;
ALTER INDEX library_items_sort RENAME TO idx_library_items_sort;
ALTER INDEX library_items_catalog RENAME TO idx_library_items_catalog;
ALTER INDEX library_items_origin RENAME TO idx_library_items_origin;
ALTER INDEX library_catalog_fetched RENAME TO idx_library_catalog_fetched;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
