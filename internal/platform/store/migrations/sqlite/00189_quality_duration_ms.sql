-- +goose Up
-- Schema naming (#2528): the two duration columns spelled their unit `_millis` where every other
-- millisecond column says `_ms`. Renames only; no value changes. The schema lint's unit rule
-- keeps the spelling from coming back.
ALTER TABLE quality_receipts RENAME COLUMN duration_millis TO duration_ms;
ALTER TABLE quality_daily_aggregates RENAME COLUMN duration_millis TO duration_ms;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
