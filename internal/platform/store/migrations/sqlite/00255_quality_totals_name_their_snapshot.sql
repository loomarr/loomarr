-- +goose Up
-- Schema to a 10 (#2793, item 1): a quality daily total names its evaluation run by foreign key.
--
-- quality_daily_aggregates.run_snapshot_id references quality_run_snapshots, as
-- quality_receipts.run_snapshot_id already does. It used '' for "ordinary use", which no snapshot
-- has as its id, so it becomes NULL. A NULL can't be part of a primary key, so a total is now
-- unique by an index over COALESCE(run_snapshot_id, ''), which keeps one ordinary total per day,
-- stage and outcome, and the rollup's upsert names that index.
--
-- No ON DELETE action: the snapshot purge already keeps every snapshot a total names. A total
-- naming a snapshot that is gone can't be told apart from another run's, so it is dropped (none
-- on the maintainer's install).
--
-- SQLite can't change a primary key in place, so the table is rebuilt. Nothing references it.

ALTER TABLE quality_daily_aggregates RENAME TO quality_daily_aggregates_old;
CREATE TABLE quality_daily_aggregates (
    day               TEXT   NOT NULL,
    stage             TEXT   NOT NULL,
    outcome           TEXT   NOT NULL CHECK (outcome IN ('succeeded', 'empty', 'failed', 'abstained',
        'accepted', 'rejected', 'approved', 'declined', 'playable', 'scheduled')),
    run_snapshot_id   TEXT   REFERENCES quality_run_snapshots(id),
    observation_count BIGINT NOT NULL,
    duration_ms       BIGINT NOT NULL,
    tool_calls        BIGINT NOT NULL,
    candidate_count   BIGINT NOT NULL,
    cost_nanos        BIGINT NOT NULL
);
INSERT INTO quality_daily_aggregates
    (day, stage, outcome, run_snapshot_id, observation_count, duration_ms, tool_calls,
     candidate_count, cost_nanos)
SELECT day, stage, outcome, NULLIF(run_snapshot_id, ''), observation_count, duration_ms, tool_calls,
       candidate_count, cost_nanos
FROM quality_daily_aggregates_old a
WHERE a.run_snapshot_id = ''
   OR EXISTS (SELECT 1 FROM quality_run_snapshots s WHERE s.id = a.run_snapshot_id);
DROP TABLE quality_daily_aggregates_old;
CREATE UNIQUE INDEX idx_quality_daily_aggregates_key
    ON quality_daily_aggregates (day, stage, outcome, COALESCE(run_snapshot_id, ''));
CREATE INDEX idx_quality_daily_aggregates_snapshot ON quality_daily_aggregates (run_snapshot_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
