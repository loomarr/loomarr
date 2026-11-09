-- +goose Up
-- Schema to a 10 (#2793, item 1): a collection's members reference their collection and their
-- title. Deleting a collection takes its members with it; a title a collection still names can't
-- be deleted, which the catalog purge already waits for.
--
-- A member whose collection or title is gone names nothing the library can show, so it is
-- dropped.
DELETE FROM library_collection_members m
WHERE NOT EXISTS (SELECT 1 FROM library_collections c WHERE c.id = m.collection_id)
   OR NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = m.inventory_item_id);
ALTER TABLE library_collection_members ADD CONSTRAINT library_collection_members_collection_id_fkey
    FOREIGN KEY (collection_id) REFERENCES library_collections(id) ON DELETE CASCADE;
ALTER TABLE library_collection_members ADD CONSTRAINT library_collection_members_inventory_item_id_fkey
    FOREIGN KEY (inventory_item_id) REFERENCES library_items(id);
CREATE INDEX idx_library_collection_members_item ON library_collection_members (inventory_item_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
