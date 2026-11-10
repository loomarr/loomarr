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

-- filler_acquisition_runs ----------------------------------------------------------------------
ALTER TABLE filler_acquisition_runs
    ALTER COLUMN source_id DROP NOT NULL, ALTER COLUMN source_id DROP DEFAULT,
    ALTER COLUMN pull_id DROP NOT NULL, ALTER COLUMN pull_id DROP DEFAULT;
UPDATE filler_acquisition_runs r SET source_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_sources s WHERE s.id = r.source_id);
UPDATE filler_acquisition_runs r SET pull_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_pulls p WHERE p.id = r.pull_id);
ALTER TABLE filler_acquisition_runs ADD CONSTRAINT filler_acquisition_runs_source_id_fkey
    FOREIGN KEY (source_id) REFERENCES filler_sources(id) ON DELETE SET NULL;
ALTER TABLE filler_acquisition_runs ADD CONSTRAINT filler_acquisition_runs_pull_id_fkey
    FOREIGN KEY (pull_id) REFERENCES filler_pulls(id) ON DELETE SET NULL;

-- filler_clip_pipeline -------------------------------------------------------------------------
ALTER TABLE filler_clip_pipeline ALTER COLUMN acquisition_id DROP NOT NULL, ALTER COLUMN acquisition_id DROP DEFAULT;
UPDATE filler_clip_pipeline p SET acquisition_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = p.acquisition_id);
ALTER TABLE filler_clip_pipeline ADD CONSTRAINT filler_clip_pipeline_acquisition_id_fkey
    FOREIGN KEY (acquisition_id) REFERENCES filler_acquisition_runs(id);

-- filler_ready_events --------------------------------------------------------------------------
ALTER TABLE filler_ready_events ALTER COLUMN acquisition_id DROP NOT NULL, ALTER COLUMN acquisition_id DROP DEFAULT;
UPDATE filler_ready_events e SET acquisition_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM filler_acquisition_runs r WHERE r.id = e.acquisition_id);
ALTER TABLE filler_ready_events ADD CONSTRAINT filler_ready_events_acquisition_id_fkey
    FOREIGN KEY (acquisition_id) REFERENCES filler_acquisition_runs(id) ON DELETE SET NULL;
CREATE INDEX idx_filler_ready_events_acquisition ON filler_ready_events (acquisition_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
