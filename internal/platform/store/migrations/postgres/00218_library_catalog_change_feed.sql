-- +goose Up
-- #3225 (#2151 decision 20): each night the catalog's change feed names the titles it changed.
-- `changed` marks a cached record the feed named, so the Enricher fetches it again ahead of the
-- 90-day refresh; saving the record clears it. `library_catalog_feeds` keeps, per catalog, when
-- its feed was last read, so a restart picks up where it left off.
ALTER TABLE library_catalog ADD COLUMN changed BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE library_catalog_feeds (
    provider TEXT PRIMARY KEY,
    read_at_ms BIGINT NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
