-- +goose Up
-- Scheduler state (§18.1): one row per named, recurring background job. This table is
-- RUNTIME STATE, not config — it records when each job last ran, whether it succeeded, and
-- when it is next due. The set of jobs and their intervals are code + settings (`job.<name>.
-- interval`); rows here are reconciled from the code registry on boot (missing → created,
-- unknown-in-code → ignored).
--
-- `next_run` is the schedule lease: the heartbeat claims a job by advancing next_run so a
-- concurrent tick/replica won't re-run it (SQLite guarded UPDATE; Postgres SKIP LOCKED). A
-- "Run now" sets next_run <= now. Times are epoch seconds, matching the rest of the store.
--
-- Forward-only (§16). Tenth migration; scheduled_jobs holds only regenerable state, but we
-- still ALTER/create in place, never drop a table with real rows on downgrade paths.
CREATE TABLE scheduled_jobs (
    name        TEXT PRIMARY KEY,
    last_run    BIGINT NOT NULL DEFAULT 0,
    last_result TEXT   NOT NULL DEFAULT '',
    last_error  TEXT   NOT NULL DEFAULT '',
    next_run    BIGINT NOT NULL DEFAULT 0,
    updated_at  BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_scheduled_jobs_next_run ON scheduled_jobs (next_run);

-- +goose Down
DROP TABLE scheduled_jobs;
