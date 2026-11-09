-- +goose Up
-- Schema to a 10 (#2793, item 1): a collection's members reference their collection and their
-- title. Deleting a collection takes its members with it; a title a collection still names can't
-- be deleted, which the catalog purge already waits for.
--
-- A member whose collection or title is gone names nothing the library can show, so it is
-- dropped. SQLite cannot add a foreign key in place, so the table is rebuilt.
ALTER TABLE library_collection_members RENAME TO library_collection_members_old;
CREATE TABLE library_collection_members (
    collection_id     TEXT    NOT NULL REFERENCES library_collections(id) ON DELETE CASCADE,
    inventory_item_id TEXT    NOT NULL REFERENCES library_items(id),
    position          INTEGER NOT NULL,
    PRIMARY KEY (collection_id, inventory_item_id)
);
INSERT INTO library_collection_members (collection_id, inventory_item_id, position)
SELECT m.collection_id, m.inventory_item_id, m.position
FROM library_collection_members_old m
WHERE EXISTS (SELECT 1 FROM library_collections c WHERE c.id = m.collection_id)
  AND EXISTS (SELECT 1 FROM library_items i WHERE i.id = m.inventory_item_id);
DROP TABLE library_collection_members_old;
CREATE INDEX idx_library_collection_members_item ON library_collection_members (inventory_item_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
