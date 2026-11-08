-- +goose Up
-- Schema to a 9 (#2528), integrity step 2: the state columns of tables other tables reference accept
-- only the values the code writes, and channels loses filler_ref, which nothing ever set. The SQLite
-- twin rebuilds each table with its descendants; Postgres changes them in place.

ALTER TABLE channels DROP COLUMN filler_ref;
ALTER TABLE channels ADD CONSTRAINT channels_status_check
    CHECK (status IN ('building', 'live', 'drifted', 'empty', 'detached', 'paused'));
ALTER TABLE filler_acquisition_runs ADD CONSTRAINT filler_acquisition_runs_status_check
    CHECK (status IN ('queued', 'running', 'success', 'error'));
ALTER TABLE filler_pulls ADD CONSTRAINT filler_pulls_status_check
    CHECK (status IN ('pending', 'approved', 'dismissed'));
ALTER TABLE filler_sources ADD CONSTRAINT filler_sources_kind_check
    CHECK (kind IN ('archive', 'youtube', 'folder', 'library'));
ALTER TABLE inventory_sources ADD CONSTRAINT inventory_sources_kind_check
    CHECK (kind IN ('library_original', 'local_file'));
ALTER TABLE library_items ADD CONSTRAINT library_items_kind_check
    CHECK (kind IN ('movie', 'series', 'episode'));
ALTER TABLE proposal_jobs ADD CONSTRAINT proposal_jobs_kind_check
    CHECK (kind IN ('suggest', 'recurate'));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
