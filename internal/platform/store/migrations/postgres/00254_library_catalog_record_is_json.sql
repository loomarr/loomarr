-- +goose Up
-- Schema to a 10 (#2793): library_catalog.record, the catalog's record of a title, is JSON, so it
-- is jsonb like every JSON column (00192), the twin of SQLite's TEXT with CHECK (json_valid(...)).
-- It used '' both before the record is fetched and when the catalog has no such title; '' is not
-- JSON, so it becomes NULL. fetched_at_ms still tells the two apart.
ALTER TABLE library_catalog
    ALTER COLUMN record DROP DEFAULT,
    ALTER COLUMN record DROP NOT NULL,
    ALTER COLUMN record TYPE jsonb USING NULLIF(record, '')::jsonb;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
