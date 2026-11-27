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

DELETE FROM quality_daily_aggregates a
WHERE a.run_snapshot_id <> ''
  AND NOT EXISTS (SELECT 1 FROM quality_run_snapshots s WHERE s.id = a.run_snapshot_id);
ALTER TABLE quality_daily_aggregates DROP CONSTRAINT quality_daily_aggregates_pkey;
ALTER TABLE quality_daily_aggregates
    ALTER COLUMN run_snapshot_id DROP NOT NULL,
    ALTER COLUMN run_snapshot_id DROP DEFAULT;
UPDATE quality_daily_aggregates SET run_snapshot_id = NULL WHERE run_snapshot_id = '';
ALTER TABLE quality_daily_aggregates ADD CONSTRAINT quality_daily_aggregates_run_snapshot_id_fkey
    FOREIGN KEY (run_snapshot_id) REFERENCES quality_run_snapshots(id);
CREATE UNIQUE INDEX idx_quality_daily_aggregates_key
    ON quality_daily_aggregates (day, stage, outcome, COALESCE(run_snapshot_id, ''));
CREATE INDEX idx_quality_daily_aggregates_snapshot ON quality_daily_aggregates (run_snapshot_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
