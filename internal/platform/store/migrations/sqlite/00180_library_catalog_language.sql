-- +goose Up
-- The language each cached catalog record was read in (#2151 decision 21), such as "en-US". A
-- record in another language than the household's is fetched again; one saved before this has
-- none, so every record is read once more.
ALTER TABLE library_catalog ADD COLUMN language TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
