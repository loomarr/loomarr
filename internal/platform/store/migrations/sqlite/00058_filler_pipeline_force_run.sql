-- +goose Up
-- An operator-requested stage rerun must survive a process restart. Without this bit the worker
-- can apply the rung's ordinary shortcut and turn "run it again" into a visible no-op.
ALTER TABLE filler_clip_pipeline ADD COLUMN force_run INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
