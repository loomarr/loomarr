-- +goose Up
-- A cached catalog record notes the shape it was fetched in (#2151 decision 53). When the scanner
-- reads more from a catalog than it used to, records saved before are fetched again first, as if
-- never fetched, instead of waiting for the refresh. Every record held now is version 0.
ALTER TABLE library_catalog ADD COLUMN version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
