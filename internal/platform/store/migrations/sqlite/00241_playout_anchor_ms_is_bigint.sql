-- +goose Up
-- #4140: the Postgres twin of this migration makes `channels.playout_anchor_ms` BIGINT. SQLite's
-- INTEGER is already 64 bits, so nothing changes here; the file keeps the two sets in step.
SELECT 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
