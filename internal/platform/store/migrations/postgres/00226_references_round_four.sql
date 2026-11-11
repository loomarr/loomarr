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

-- settings.updated_by ---------------------------------------------------------------------------
UPDATE settings s SET updated_by = NULL
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = s.updated_by);
ALTER TABLE settings ADD CONSTRAINT settings_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_settings_updated_by ON settings (updated_by);

-- filler_acquisition_runs.pull_id ---------------------------------------------------------------
ALTER TABLE filler_acquisition_runs ALTER COLUMN pull_id DROP NOT NULL, ALTER COLUMN pull_id DROP DEFAULT;
UPDATE filler_acquisition_runs r SET pull_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_pulls p WHERE p.id = r.pull_id);
ALTER TABLE filler_acquisition_runs ADD CONSTRAINT filler_acquisition_runs_pull_id_fkey
    FOREIGN KEY (pull_id) REFERENCES filler_pulls(id) ON DELETE SET NULL;

-- filler_clip_pipeline.acquisition_id -----------------------------------------------------------
ALTER TABLE filler_clip_pipeline ALTER COLUMN acquisition_id DROP NOT NULL, ALTER COLUMN acquisition_id DROP DEFAULT;
UPDATE filler_clip_pipeline p SET acquisition_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = p.acquisition_id);
ALTER TABLE filler_clip_pipeline ADD CONSTRAINT filler_clip_pipeline_acquisition_id_fkey
    FOREIGN KEY (acquisition_id) REFERENCES filler_acquisition_runs(id);

-- filler_ready_events.acquisition_id ------------------------------------------------------------
ALTER TABLE filler_ready_events ALTER COLUMN acquisition_id DROP NOT NULL, ALTER COLUMN acquisition_id DROP DEFAULT;
UPDATE filler_ready_events e SET acquisition_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = e.acquisition_id);
ALTER TABLE filler_ready_events ADD CONSTRAINT filler_ready_events_acquisition_id_fkey
    FOREIGN KEY (acquisition_id) REFERENCES filler_acquisition_runs(id);
CREATE INDEX idx_filler_ready_events_acquisition ON filler_ready_events (acquisition_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
