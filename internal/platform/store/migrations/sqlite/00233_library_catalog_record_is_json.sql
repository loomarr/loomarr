-- +goose Up
-- Schema to a 10 (#2793): library_catalog.record, the catalog's record of a title, is JSON, so it
-- is declared so (CHECK (json_valid(...)), 00192), the twin of Postgres's jsonb. It used '' both
-- before the record is fetched and when the catalog has no such title; '' is not JSON, so it
-- becomes NULL. fetched_at_ms still tells the two apart.
--
-- SQLite cannot add a CHECK to a column in place, so a checked column takes the values and the
-- old one is dropped under its name (the 00211 pattern).
ALTER TABLE library_catalog ADD COLUMN record_doc TEXT CHECK (json_valid(record_doc));
UPDATE library_catalog SET record_doc = NULLIF(record, '');
ALTER TABLE library_catalog DROP COLUMN record;
ALTER TABLE library_catalog RENAME COLUMN record_doc TO record;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
