-- +goose Up
-- Per-job pause (§18.1). An operator can stop a task from running on its schedule without
-- editing its cron to something that never fires, and without the task disappearing from the
-- Tasks page.
--
-- WHY A COLUMN AND NOT A SETTING. Schedules live in the settings registry (`job.<name>.schedule`),
-- so a matching `job.<name>.paused` key looks like the consistent choice. But the settings
-- registry requires every key to be DECLARED up front, which means twelve static entries that
-- duplicate the job registry — and a job added later would have no key, so pausing it would
-- 404 with nothing on screen explaining why. Pause is per-job runtime state, and
-- `scheduled_jobs` is already the per-job runtime state table (last run, last result, next
-- run); one column here works for every job that is ever registered.
--
-- ⚠ NOT the same thing as `DisabledReason`. Disabled states a fact about the environment
-- (backup cannot run on Postgres) that no amount of clicking changes and that has no Resume
-- control. Paused is an operator's decision, and is theirs to undo.
--
-- ⚠ `paused` must NOT appear in UpsertScheduledJob's ON CONFLICT DO UPDATE list. That upsert
-- runs after EVERY execution, so including it would reset the flag on the next run of any job
-- the operator paused — the same shape as the `env_override` trap recorded in §3.1, where an
-- ordinary save silently re-locked the key at the one moment the operator was editing it.
-- Pause is written only by its own explicit call.
ALTER TABLE scheduled_jobs ADD COLUMN paused BOOLEAN NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE scheduled_jobs DROP COLUMN paused;
