-- +goose Up
-- #1254: source checks are claimed independently from the scheduler's whole-job lease because a
-- person may press "Look for new clips" while the scheduled pass is active. Retry state is
-- durable so a restart cannot turn an unhealthy provider into one request per minute.

ALTER TABLE filler_sources ADD COLUMN check_failure_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_sources ADD COLUMN check_retry_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_sources ADD COLUMN check_lease_until INTEGER NOT NULL DEFAULT 0;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
