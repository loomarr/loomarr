-- +goose Up
-- #4476: a home video's poster is a frame of its file. A source row records the inventory's
-- revision of the file, and a poster ref the file its frame came from ("" for any other ref).
ALTER TABLE library_sources ADD COLUMN revision TEXT NOT NULL DEFAULT '';
ALTER TABLE library_item_images ADD COLUMN source TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
