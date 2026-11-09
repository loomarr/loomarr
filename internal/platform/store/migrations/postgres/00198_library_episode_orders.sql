-- +goose Up
-- #2151 step 14b-5, decision 53: which order a show's episode files follow, as the household
-- answered in Needs you, by the show's catalog row ('<provider>:series:<id>'): the catalog's own
-- seasons, one of its other orders, or the names and numbers in the files. Kept apart from the
-- catalog cache, which retention clears, so an answer outlives its records.
CREATE TABLE library_episode_orders (
    series_catalog TEXT   NOT NULL PRIMARY KEY,
    choice         TEXT   NOT NULL CHECK (choice IN ('catalog', 'order', 'files')),
    order_id       TEXT   NOT NULL DEFAULT '',
    answered_at_ms BIGINT NOT NULL,
    CHECK ((choice = 'order') = (order_id <> ''))
);

-- An episode the scanner could not number by its show's catalog and nobody has answered for: it
-- keeps its file's name and number, and its show is in Needs you.
ALTER TABLE library_items ADD COLUMN unnumbered BOOLEAN NOT NULL DEFAULT FALSE;

-- The catalog cache also holds a show's other episode orders, fetched by the order's id.
ALTER TABLE library_catalog DROP CONSTRAINT library_catalog_kind_check;
ALTER TABLE library_catalog ADD CONSTRAINT library_catalog_kind_check
    CHECK (kind IN ('movie', 'series', 'episode', 'season', 'collection', 'episode_order'));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
