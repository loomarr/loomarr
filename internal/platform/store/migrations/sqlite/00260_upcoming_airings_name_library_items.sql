-- +goose Up
-- Schema to a 10 (#2793, item 1): channel_upcoming_airings.library_item_id references
-- library_items.
--
-- Unlike airings (00259), an upcoming airing goes with its item: the table is a forecast each
-- reconcile replaces whole, so it never keeps an item the catalog is done with. A row naming no
-- item is dropped, and the next reconcile writes the channel's rows again.
ALTER TABLE channel_upcoming_airings RENAME TO channel_upcoming_airings_old;
DROP INDEX idx_channel_upcoming_airings_item;
DROP INDEX idx_channel_upcoming_airings_title;
CREATE TABLE channel_upcoming_airings (
    channel_id      TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    starts_at_ms    BIGINT  NOT NULL,
    ends_at_ms      BIGINT  NOT NULL,
    library_item_id TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    title_key       TEXT    NOT NULL,
    forecast        BOOLEAN NOT NULL CHECK (forecast IN (0, 1)),
    revision        BIGINT  NOT NULL,
    PRIMARY KEY (channel_id, starts_at_ms)
);
INSERT INTO channel_upcoming_airings
    (channel_id, starts_at_ms, ends_at_ms, library_item_id, title_key, forecast, revision)
SELECT channel_id, starts_at_ms, ends_at_ms, library_item_id, title_key, forecast, revision
FROM channel_upcoming_airings_old u
WHERE EXISTS (SELECT 1 FROM library_items i WHERE i.id = u.library_item_id);
DROP TABLE channel_upcoming_airings_old;
CREATE INDEX idx_channel_upcoming_airings_item ON channel_upcoming_airings (library_item_id, ends_at_ms);
CREATE INDEX idx_channel_upcoming_airings_title ON channel_upcoming_airings (title_key, ends_at_ms);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
