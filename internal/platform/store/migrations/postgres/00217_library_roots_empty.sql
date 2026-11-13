-- +goose Up
-- #2151: a folder that opens but holds nothing is usually a share that isn't mounted (the mount
-- point is an empty folder). `empty` records that from the same check that sets `readable`, so
-- Settings → Library can say so instead of calling the library caught up.
ALTER TABLE library_roots ADD COLUMN empty BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
