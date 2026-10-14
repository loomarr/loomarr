-- +goose Up
-- V52 phase 7 (§22). Postgres mirror of the sqlite 00048; the full reasoning lives there.
--
-- In short: the fetch job re-keys an adopted row from a hash-of-the-URL onto the content hash and
-- deletes the placeholder, which left an adopted URL unfindable once it succeeded — so re-adopting
-- it re-downloaded bytes already on disk. Phase 7's icon picker made that a per-open cost. This
-- indexes the column the lookup uses.
--
-- ⚠ Not unique: the re-key writes the new row before deleting the old one, so two rows share a
-- source URL for that window by design. The lookup asks only for FETCHED rows instead.
CREATE INDEX idx_images_source_url ON images (source_url);

-- +goose Down
DROP INDEX IF EXISTS idx_images_source_url;
