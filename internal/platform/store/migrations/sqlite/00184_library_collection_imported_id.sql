-- +goose Up
-- The media-server collection a household collection was brought over from (#2151 step 15): the
-- cutover rewrites a channel scope that names the media server's collection to the household's,
-- and imports each media-server collection once. Empty when made in Loomarr.
ALTER TABLE library_collections ADD COLUMN imported_id TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
