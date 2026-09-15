-- +goose Up
-- #1254: automatic-download cadence is evaluated per source. A successful check must remain
-- durable even when it finds nothing new, otherwise the minute-level planner would repeatedly
-- poll an empty collection until one item happened to arrive.

ALTER TABLE filler_sources ADD COLUMN last_checked_at INTEGER NOT NULL DEFAULT 0;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
