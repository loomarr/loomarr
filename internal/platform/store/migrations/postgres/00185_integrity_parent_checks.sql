-- +goose Up
-- Schema to a 9 (#2528), integrity step 2: the state columns of tables other tables reference accept
-- only the values the code writes; channels.filler_ref and clips.rating, which nothing ever set, go,
-- and so does the dead spoken-safety ledger. The SQLite
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
-- Art an arr named in its own words (headshot, unknown) goes; the next scan writes mapped art.
DELETE FROM library_art WHERE kind NOT IN ('poster', 'fanart', 'clearlogo', 'banner', 'landscape',
    'clearart', 'discart', 'thumb', 'season-poster');
ALTER TABLE library_art ADD CONSTRAINT library_art_kind_check
    CHECK (kind IN ('poster', 'fanart', 'clearlogo', 'banner', 'landscape', 'clearart', 'discart',
        'thumb', 'season-poster'));
ALTER TABLE proposal_jobs ADD CONSTRAINT proposal_jobs_kind_check
    CHECK (kind IN ('suggest', 'recurate'));

-- The spoken-safety ledger: nothing has started a run since its evaluation cascade was deleted
-- (#1763). Events go first: they reference the runs.
DROP TABLE filler_spoken_safety_events;
DROP TABLE filler_spoken_safety_runs;
ALTER TABLE clips DROP COLUMN rating;
ALTER TABLE filler_inference_evaluations
    ADD CONSTRAINT filler_inference_evaluations_state_check
        CHECK (state IN ('reserved', 'completed', 'failed', 'held_budget')),
    ADD CONSTRAINT filler_inference_evaluations_outcome_check
        CHECK (outcome IN ('', 'window_complete'));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
