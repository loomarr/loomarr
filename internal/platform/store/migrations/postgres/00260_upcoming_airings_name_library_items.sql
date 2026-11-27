-- +goose Up
-- Schema to a 10 (#2793, item 1): channel_upcoming_airings.library_item_id references
-- library_items.
--
-- Unlike airings (00259), an upcoming airing goes with its item: the table is a forecast each
-- reconcile replaces whole, so it never keeps an item the catalog is done with. A row naming no
-- item is dropped, and the next reconcile writes the channel's rows again.
DELETE FROM channel_upcoming_airings u
WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = u.library_item_id);
ALTER TABLE channel_upcoming_airings ADD CONSTRAINT channel_upcoming_airings_library_item_id_fkey
    FOREIGN KEY (library_item_id) REFERENCES library_items(id) ON DELETE CASCADE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
