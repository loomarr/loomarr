-- +goose Up
-- V52 phase 6 (§22). Postgres mirror of the sqlite 00046; the full reasoning lives there.
--
-- In short: a clip's still and hover loop become image-service images, and the forward pointer
-- lives on the clip row — `image_refs` is the GC's reverse index, not the lookup. `thumbnail` and
-- `preview` stay for now because the existing routes still serve them and the phase-8 backfill
-- reads them to find artwork that predates this migration.
ALTER TABLE clips ADD COLUMN thumb_image_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN hover_image_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE clips DROP COLUMN thumb_image_hash;
ALTER TABLE clips DROP COLUMN hover_image_hash;
