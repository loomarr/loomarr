-- +goose Up
-- Schema naming (#2323): the rest of the time columns whose unit their name hid (#2319). Renames
-- only; no value changes.
--
-- `_at` is unix seconds, `_at_ms` unix milliseconds and `_at_ns` unix nanoseconds. Every column
-- here holds milliseconds except `discovery_feedback.created_at`, which holds nanoseconds; it keeps
-- them, because two answers in the same millisecond still order by when they were given.
ALTER TABLE user_favourite_channels RENAME COLUMN added_at TO added_at_ms;
ALTER TABLE user_recent_channels RENAME COLUMN tuned_at TO tuned_at_ms;
ALTER TABLE user_hidden_ideas RENAME COLUMN hidden_at TO hidden_at_ms;
ALTER TABLE client_crash_reports RENAME COLUMN received_at TO received_at_ms;
ALTER TABLE client_crash_reports RENAME COLUMN occurred_at TO occurred_at_ms;
ALTER TABLE diagnostic_events RENAME COLUMN occurred_at TO occurred_at_ms;
ALTER TABLE diagnostic_events RENAME COLUMN received_at TO received_at_ms;
ALTER TABLE diagnostic_process_runs RENAME COLUMN started_at TO started_at_ms;
ALTER TABLE diagnostic_process_runs RENAME COLUMN ended_at TO ended_at_ms;
ALTER TABLE diagnostic_process_runs RENAME COLUMN updated_at TO updated_at_ms;
ALTER TABLE filler_airings RENAME COLUMN aired_at TO aired_at_ms;
ALTER TABLE filler_airings RENAME COLUMN recorded_at TO recorded_at_ms;
ALTER TABLE discovery_feedback RENAME COLUMN created_at TO created_at_ns;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
