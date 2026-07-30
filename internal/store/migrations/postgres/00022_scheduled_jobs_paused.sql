-- +goose Up
-- Per-job pause (§18.1). Postgres twin of the SQLite migration — see that file for why this
-- is a column on scheduled_jobs rather than a settings key, and why `paused` must never join
-- UpsertScheduledJob's ON CONFLICT DO UPDATE list.
ALTER TABLE scheduled_jobs ADD COLUMN paused BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE scheduled_jobs DROP COLUMN paused;
