-- +goose Up
-- Schema to a 10 (#2793, item 1): the library's links between its own rows are foreign keys.
--
--   - library_items.series_id, an episode's show, references library_items;
--   - library_files.inventory_item_id and inventory_series_id, the title and show a file was last
--     written as, reference library_items;
--   - library_candidates.inventory_item_id references library_items and goes with its title.
--
-- The three links used '' for "not known yet", which no row has as its id, so they become nullable
-- and each '' turns into NULL. A link naming no row is cleared the same way: the next scan writes
-- it again. A candidate whose title is gone is dropped. Nothing deletes a title a file or an
-- episode still names (the catalog purge checks), so a restricting key changes no behaviour.
ALTER TABLE library_items ALTER COLUMN series_id DROP NOT NULL, ALTER COLUMN series_id DROP DEFAULT;
UPDATE library_items e SET series_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM library_items s WHERE s.id = e.series_id);
ALTER TABLE library_items ADD CONSTRAINT library_items_series_id_fkey
    FOREIGN KEY (series_id) REFERENCES library_items(id);

ALTER TABLE library_files
    ALTER COLUMN inventory_item_id DROP NOT NULL, ALTER COLUMN inventory_item_id DROP DEFAULT,
    ALTER COLUMN inventory_series_id DROP NOT NULL, ALTER COLUMN inventory_series_id DROP DEFAULT;
UPDATE library_files f SET inventory_item_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = f.inventory_item_id);
UPDATE library_files f SET inventory_series_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = f.inventory_series_id);
ALTER TABLE library_files ADD CONSTRAINT library_files_inventory_item_id_fkey
    FOREIGN KEY (inventory_item_id) REFERENCES library_items(id);
ALTER TABLE library_files ADD CONSTRAINT library_files_inventory_series_id_fkey
    FOREIGN KEY (inventory_series_id) REFERENCES library_items(id);

DELETE FROM library_candidates c
WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = c.inventory_item_id);
ALTER TABLE library_candidates ADD CONSTRAINT library_candidates_inventory_item_id_fkey
    FOREIGN KEY (inventory_item_id) REFERENCES library_items(id) ON DELETE CASCADE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
