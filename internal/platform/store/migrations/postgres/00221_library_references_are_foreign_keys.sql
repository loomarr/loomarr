-- +goose Up
-- Schema to a 10 (#2793, item 1): the library items a title and an airing name are foreign keys.
--
--   - titles.library_id, the library item a title is, references library_items. It used '' for
--     "not in the library", which no item has as its id, so it becomes NULL;
--   - airings.library_item_id and scheduled_airings.library_item_id, what aired or is scheduled,
--     reference library_items.
--
-- Neither deletes with the item (no ON DELETE action): the catalog's retention already keeps a
-- library row while any stored record names it (PurgeSpentCatalog, #2528), and the keys make the
-- database hold that too.
--
-- The #2151 cutover moved every stored media-server id to its library id where the media server
-- still knew it. What is left names media the server no longer has, so it can never be a library
-- item: such a title has no library item (NULL), and such an airing is dropped. Every live airing
-- already names a library item; the dropped ones are older history of titles that are gone.

-- titles ---------------------------------------------------------------------------------------
ALTER TABLE titles ALTER COLUMN library_id DROP NOT NULL, ALTER COLUMN library_id DROP DEFAULT;
UPDATE titles t SET library_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = t.library_id);
ALTER TABLE titles ADD CONSTRAINT titles_library_id_fkey
    FOREIGN KEY (library_id) REFERENCES library_items(id);

-- airings and scheduled_airings ----------------------------------------------------------------
DELETE FROM airings a WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = a.library_item_id);
DELETE FROM scheduled_airings a WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = a.library_item_id);
ALTER TABLE airings ADD CONSTRAINT airings_library_item_id_fkey
    FOREIGN KEY (library_item_id) REFERENCES library_items(id);
ALTER TABLE scheduled_airings ADD CONSTRAINT scheduled_airings_library_item_id_fkey
    FOREIGN KEY (library_item_id) REFERENCES library_items(id);
CREATE INDEX idx_scheduled_airings_library_item ON scheduled_airings (library_item_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
