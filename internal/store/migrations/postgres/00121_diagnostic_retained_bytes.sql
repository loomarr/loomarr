-- +goose Up
-- #1398: diagnostic_events reached 2.7M rows / 11 indexes / 1.7 GB of a 1.8 GB database.
-- Two changes, both forward-only (§16):
--
-- 1. diagnostic_retained_bytes is the running total of size_bytes per evidence table. The store
--    maintains it in the SAME transaction as every insert/upsert/delete, so retention reads two
--    rows instead of SUM()-scanning millions. Seeded once here from the existing rows.
-- 2. Index diet. Every diagnostic_events query is time-bounded (idx_diagnostic_events_time), so an
--    index only earns its write cost by making a selective filter cheap. `source` (3 values) and
--    `instance_id` (one value per install) never do, so they go. The six correlation-id indexes
--    stay but become PARTIAL (non-empty values only): most rows carry an empty id in most of these
--    columns, and an empty value was indexed on every insert for nothing.
--
-- ONE-TIME STARTUP COST: on the household install (~2.7M diagnostic_events rows) this migration
-- scans the table once to seed the total and rebuilds six indexes. Expect the first boot after
-- upgrade to take noticeably longer (tens of seconds to minutes) with no log output; it is not a
-- hang. It runs once. On SQLite the file does not shrink until a VACUUM; freed pages are reused.
CREATE TABLE diagnostic_retained_bytes (
    scope TEXT   NOT NULL PRIMARY KEY,
    bytes BIGINT NOT NULL DEFAULT 0
);
INSERT INTO diagnostic_retained_bytes (scope, bytes)
    SELECT 'events', COALESCE(SUM(size_bytes), 0) FROM diagnostic_events;
INSERT INTO diagnostic_retained_bytes (scope, bytes)
    SELECT 'process_runs', COALESCE(SUM(size_bytes), 0) FROM diagnostic_process_runs;

DROP INDEX IF EXISTS idx_diagnostic_events_source_time;
DROP INDEX IF EXISTS idx_diagnostic_events_instance;

DROP INDEX IF EXISTS idx_diagnostic_events_request;
DROP INDEX IF EXISTS idx_diagnostic_events_playback;
DROP INDEX IF EXISTS idx_diagnostic_events_channel;
DROP INDEX IF EXISTS idx_diagnostic_events_schedule_block;
DROP INDEX IF EXISTS idx_diagnostic_events_job;
DROP INDEX IF EXISTS idx_diagnostic_events_process;
CREATE INDEX idx_diagnostic_events_request ON diagnostic_events (request_id, occurred_at DESC) WHERE request_id <> '';
CREATE INDEX idx_diagnostic_events_playback ON diagnostic_events (playback_session_id, occurred_at DESC) WHERE playback_session_id <> '';
CREATE INDEX idx_diagnostic_events_channel ON diagnostic_events (channel_id, occurred_at DESC) WHERE channel_id <> '';
CREATE INDEX idx_diagnostic_events_schedule_block ON diagnostic_events (schedule_block_id, occurred_at DESC) WHERE schedule_block_id <> '';
CREATE INDEX idx_diagnostic_events_job ON diagnostic_events (job_id, occurred_at DESC) WHERE job_id <> '';
CREATE INDEX idx_diagnostic_events_process ON diagnostic_events (process_run_id, occurred_at DESC) WHERE process_run_id <> '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
