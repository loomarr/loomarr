-- +goose Up
-- Schema to a 10 (#2793, item 1): every reference is a foreign key. Four more:
--   - settings.updated_by, the admin who last changed a setting, is forgotten when that admin is
--     deleted (SET NULL); the value stays. A name of a person already gone becomes NULL.
--   - filler_acquisition_runs.pull_id, the approved pull that started a download, is forgotten
--     when the pull is purged (SET NULL), which the nightly purge does just before the run itself.
--   - filler_clip_pipeline.acquisition_id and filler_ready_events.acquisition_id, the download a
--     clip came from, keep that run (no action). The purge already kept a run a clip's conveyor
--     names, and now also one its ready event names, since a ready event never changes.
-- Each used '' for "none", which becomes NULL.
--
-- filler_acquisition_runs.source_id and filler_acquisition_artifacts.source_id stay plain text on
-- purpose: they record which source a download came from, and must outlive that source without
-- failing a download still in flight when it is deleted.
--
-- None of these columns has a CHECK, so each is moved in place (the 00211 pattern), with its index
-- dropped first and made again on the new column.

-- settings.updated_by ---------------------------------------------------------------------------
ALTER TABLE settings ADD COLUMN updated_by_ref TEXT REFERENCES users(id) ON DELETE SET NULL;
UPDATE settings SET updated_by_ref = updated_by
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = settings.updated_by);
ALTER TABLE settings DROP COLUMN updated_by;
ALTER TABLE settings RENAME COLUMN updated_by_ref TO updated_by;
CREATE INDEX idx_settings_updated_by ON settings (updated_by);

-- filler_acquisition_runs.pull_id ---------------------------------------------------------------
DROP INDEX idx_filler_acquisition_runs_pull;
ALTER TABLE filler_acquisition_runs ADD COLUMN pull_ref TEXT REFERENCES filler_pulls(id) ON DELETE SET NULL;
UPDATE filler_acquisition_runs SET pull_ref = pull_id
WHERE EXISTS (SELECT 1 FROM filler_pulls p WHERE p.id = filler_acquisition_runs.pull_id);
ALTER TABLE filler_acquisition_runs DROP COLUMN pull_id;
ALTER TABLE filler_acquisition_runs RENAME COLUMN pull_ref TO pull_id;
CREATE INDEX idx_filler_acquisition_runs_pull ON filler_acquisition_runs (pull_id, started_at DESC);

-- filler_clip_pipeline.acquisition_id -----------------------------------------------------------
DROP INDEX idx_filler_clip_pipeline_acquisition;
ALTER TABLE filler_clip_pipeline ADD COLUMN acquisition_ref TEXT REFERENCES filler_acquisition_runs(id);
UPDATE filler_clip_pipeline SET acquisition_ref = acquisition_id
WHERE EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = filler_clip_pipeline.acquisition_id);
ALTER TABLE filler_clip_pipeline DROP COLUMN acquisition_id;
ALTER TABLE filler_clip_pipeline RENAME COLUMN acquisition_ref TO acquisition_id;
CREATE INDEX idx_filler_clip_pipeline_acquisition ON filler_clip_pipeline (acquisition_id);

-- filler_ready_events.acquisition_id ------------------------------------------------------------
ALTER TABLE filler_ready_events ADD COLUMN acquisition_ref TEXT REFERENCES filler_acquisition_runs(id);
UPDATE filler_ready_events SET acquisition_ref = acquisition_id
WHERE EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = filler_ready_events.acquisition_id);
ALTER TABLE filler_ready_events DROP COLUMN acquisition_id;
ALTER TABLE filler_ready_events RENAME COLUMN acquisition_ref TO acquisition_id;
CREATE INDEX idx_filler_ready_events_acquisition ON filler_ready_events (acquisition_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
