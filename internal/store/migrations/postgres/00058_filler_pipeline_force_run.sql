-- +goose Up
-- Postgres mirror of sqlite 00058: preserve an explicit operator rerun across worker restarts.
ALTER TABLE filler_clip_pipeline ADD COLUMN force_run BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
