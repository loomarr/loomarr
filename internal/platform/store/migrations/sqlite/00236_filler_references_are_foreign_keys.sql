-- +goose Up
-- Schema to a 10 (#2793, item 1): the filler download records name their sources, pulls and runs
-- by foreign key.
--
--   - filler_acquisition_runs.source_id references filler_sources and pull_id references
--     filler_pulls. A run is history, so deleting its source forgets where it came from rather
--     than the run (SET NULL), and the nightly purge deletes an approved pull just before its run;
--   - filler_clip_pipeline.acquisition_id, the run that brought a clip in, references
--     filler_acquisition_runs with no ON DELETE action: the purge already keeps a run while a
--     pipeline row names it, and the key makes the database hold that too;
--   - filler_ready_events.acquisition_id references filler_acquisition_runs. A ready event can
--     outlive its run (it stays 30 days after its clip leaves), so it forgets the run (SET NULL).
--
-- Each link used '' for "none", which no row has as its id, so it becomes NULL, as does a link
-- naming a row that is gone (on the maintainer's install: 14 runs whose source was deleted).
--
-- Left as they are, on purpose: filler_acquisition_artifacts.source_id is part of the "already
-- downloaded" identity that outlives its source; filler_curated_offers.source_id remembers a
-- curated source the operator deleted; filler_split_shadow_decisions.proposal_id outlives its
-- proposal for 180 days.
--
-- SQLite adds a foreign key to an existing column by adding a keyed column, copying the values
-- across, and dropping the old one under its name: no table is rebuilt.

-- filler_acquisition_runs ----------------------------------------------------------------------
ALTER TABLE filler_acquisition_runs ADD COLUMN source_ref TEXT REFERENCES filler_sources(id) ON DELETE SET NULL;
UPDATE filler_acquisition_runs SET source_ref = source_id
WHERE EXISTS (SELECT 1 FROM filler_sources s WHERE s.id = filler_acquisition_runs.source_id);
DROP INDEX idx_filler_acquisition_runs_source;
ALTER TABLE filler_acquisition_runs DROP COLUMN source_id;
ALTER TABLE filler_acquisition_runs RENAME COLUMN source_ref TO source_id;
CREATE INDEX idx_filler_acquisition_runs_source ON filler_acquisition_runs (source_id, started_at DESC);

ALTER TABLE filler_acquisition_runs ADD COLUMN pull_ref TEXT REFERENCES filler_pulls(id) ON DELETE SET NULL;
UPDATE filler_acquisition_runs SET pull_ref = pull_id
WHERE EXISTS (SELECT 1 FROM filler_pulls p WHERE p.id = filler_acquisition_runs.pull_id);
DROP INDEX idx_filler_acquisition_runs_pull;
ALTER TABLE filler_acquisition_runs DROP COLUMN pull_id;
ALTER TABLE filler_acquisition_runs RENAME COLUMN pull_ref TO pull_id;
CREATE INDEX idx_filler_acquisition_runs_pull ON filler_acquisition_runs (pull_id, started_at DESC);

-- filler_clip_pipeline -------------------------------------------------------------------------
ALTER TABLE filler_clip_pipeline ADD COLUMN acquisition_ref TEXT REFERENCES filler_acquisition_runs(id);
UPDATE filler_clip_pipeline SET acquisition_ref = acquisition_id
WHERE EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = filler_clip_pipeline.acquisition_id);
DROP INDEX idx_filler_clip_pipeline_acquisition;
ALTER TABLE filler_clip_pipeline DROP COLUMN acquisition_id;
ALTER TABLE filler_clip_pipeline RENAME COLUMN acquisition_ref TO acquisition_id;
CREATE INDEX idx_filler_clip_pipeline_acquisition ON filler_clip_pipeline (acquisition_id);

-- filler_ready_events --------------------------------------------------------------------------
ALTER TABLE filler_ready_events ADD COLUMN acquisition_ref TEXT
    REFERENCES filler_acquisition_runs(id) ON DELETE SET NULL;
UPDATE filler_ready_events SET acquisition_ref = acquisition_id
WHERE EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = filler_ready_events.acquisition_id);
ALTER TABLE filler_ready_events DROP COLUMN acquisition_id;
ALTER TABLE filler_ready_events RENAME COLUMN acquisition_ref TO acquisition_id;
CREATE INDEX idx_filler_ready_events_acquisition ON filler_ready_events (acquisition_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
