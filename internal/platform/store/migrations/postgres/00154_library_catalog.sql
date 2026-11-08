-- +goose Up
-- Library catalog records (#2151, decision 20): what a catalog (TMDB) says about a title, cached so
-- the scanner never calls it during a walk.
--
-- `library_catalog` is one row per title a catalog describes: the provider, the kind and the id it
-- is fetched by. `record` is the provider's record as JSON, '' until it is fetched, and '' after
-- the catalog said it has no such title. `fetched_at` is when it was last fetched (0 never), and
-- `retry_at` holds a failed fetch back until then.
--
-- `library_items.catalog` is the row a title's details come from ("tmdb:movie:603"; '' for none),
-- so a fetched record finds the titles to write again.
CREATE TABLE library_catalog (
    provider TEXT NOT NULL,
    kind TEXT NOT NULL,
    external_id TEXT NOT NULL,
    record TEXT NOT NULL DEFAULT '',
    fetched_at BIGINT NOT NULL DEFAULT 0,
    retry_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (provider, kind, external_id)
);

CREATE INDEX library_catalog_fetched ON library_catalog (fetched_at);

ALTER TABLE library_items ADD COLUMN catalog TEXT NOT NULL DEFAULT '';
CREATE INDEX library_items_catalog ON library_items (catalog);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
