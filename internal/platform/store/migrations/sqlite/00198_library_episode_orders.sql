-- +goose Up
-- #2151 step 14b-5, decision 53: which order a show's episode files follow, as the household
-- answered in Needs you, by the show's catalog row ('<provider>:series:<id>'): the catalog's own
-- seasons, one of its other orders, or the names and numbers in the files. Kept apart from the
-- catalog cache, which retention clears, so an answer outlives its records.
CREATE TABLE library_episode_orders (
    series_catalog TEXT    NOT NULL PRIMARY KEY,
    choice         TEXT    NOT NULL CHECK (choice IN ('catalog', 'order', 'files')),
    order_id       TEXT    NOT NULL DEFAULT '',
    answered_at_ms INTEGER NOT NULL,
    CHECK ((choice = 'order') = (order_id <> ''))
);

-- An episode the scanner could not number by its show's catalog and nobody has answered for: it
-- keeps its file's name and number, and its show is in Needs you.
ALTER TABLE library_items ADD COLUMN unnumbered BOOLEAN NOT NULL DEFAULT FALSE CHECK (unnumbered IN (0, 1));

-- The catalog cache also holds a show's other episode orders, fetched by the order's id: the kind
-- CHECK widens, which SQLite does by rebuilding the table.
DROP INDEX idx_library_catalog_fetched;
ALTER TABLE library_catalog RENAME TO library_catalog_old;
CREATE TABLE library_catalog (
    provider      TEXT    NOT NULL,
    kind          TEXT    NOT NULL CHECK (kind IN ('movie', 'series', 'episode', 'season', 'collection', 'episode_order')),
    external_id   TEXT    NOT NULL,
    record        TEXT    NOT NULL DEFAULT '',
    fetched_at_ms INTEGER NOT NULL DEFAULT 0,
    retry_at_ms   INTEGER NOT NULL DEFAULT 0,
    version       INTEGER NOT NULL DEFAULT 0,
    language      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (provider, kind, external_id)
);
INSERT INTO library_catalog (provider, kind, external_id, record, fetched_at_ms, retry_at_ms, version, language)
SELECT provider, kind, external_id, record, fetched_at_ms, retry_at_ms, version, language
  FROM library_catalog_old;
DROP TABLE library_catalog_old;
CREATE INDEX idx_library_catalog_fetched ON library_catalog (fetched_at_ms);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
