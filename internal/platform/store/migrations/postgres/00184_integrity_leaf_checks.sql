-- +goose Up
-- Schema to a 9 (#2528), integrity step 1: every foreign key leads an index, and the state columns
-- of tables nothing references accept only the values the code writes. The SQLite twin rebuilds
-- each table; Postgres adds the constraints in place.

CREATE INDEX idx_client_crash_reports_user ON client_crash_reports (user_id);
CREATE INDEX idx_device_pairings_user ON device_pairings (user_id);
CREATE INDEX idx_inventory_external_ids_item ON inventory_external_ids (item_id);
CREATE INDEX idx_password_recoveries_user ON password_recoveries (user_id);
CREATE INDEX idx_filler_acquisition_artifacts_acquisition ON filler_acquisition_artifacts (acquisition_id);
CREATE INDEX idx_quality_receipts_run_snapshot ON quality_receipts (run_snapshot_id);

ALTER TABLE diagnostic_process_runs ADD CONSTRAINT diagnostic_process_runs_status_check
    CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled', 'interrupted'));
ALTER TABLE filler_acquisition_artifacts ADD CONSTRAINT filler_acquisition_artifacts_state_check
    CHECK (state IN ('staged', 'published', 'consumed', 'repair'));
ALTER TABLE filler_clip_defects ADD CONSTRAINT filler_clip_defects_outcome_check
    CHECK (outcome IN ('pass', 'borderline', 'fail', 'undecided'));
ALTER TABLE filler_clip_pipeline ADD CONSTRAINT filler_clip_pipeline_status_check
    CHECK (status IN ('queued', 'running', 'done', 'failed', 'skipped'));
ALTER TABLE filler_curated_offers ADD CONSTRAINT filler_curated_offers_local_state_check
    CHECK (local_state IN ('', 'on', 'operator', 'off'));
ALTER TABLE filler_enrichment_axes ADD CONSTRAINT filler_enrichment_axes_state_check
    CHECK (state IN ('complete', 'unsupported', 'stale'));
-- outcome only ever held 'ready' (every row is a clip becoming ready).
ALTER TABLE filler_ready_events DROP COLUMN outcome;
ALTER TABLE interactive_operations
    ADD CONSTRAINT interactive_operations_kind_check CHECK (kind IN ('filler_split', 'llm_pull')),
    ADD CONSTRAINT interactive_operations_status_check CHECK (status IN ('queued', 'running', 'success', 'error'));
ALTER TABLE library_answers ADD CONSTRAINT library_answers_kind_check
    CHECK (kind IN ('movie', 'series'));
ALTER TABLE library_candidates ADD CONSTRAINT library_candidates_kind_check
    CHECK (kind IN ('movie', 'series'));
ALTER TABLE library_catalog ADD CONSTRAINT library_catalog_kind_check
    CHECK (kind IN ('movie', 'series', 'episode', 'season', 'collection'));
ALTER TABLE library_files ADD CONSTRAINT library_files_kind_check
    CHECK (kind IN ('', 'movie', 'episode', 'series'));
ALTER TABLE library_filmography ADD CONSTRAINT library_filmography_kind_check
    CHECK (kind IN ('movie', 'series'));
ALTER TABLE library_id_map ADD CONSTRAINT library_id_map_kind_check
    CHECK (kind IN ('movie', 'series', 'episode'));
ALTER TABLE library_markers ADD CONSTRAINT library_markers_kind_check
    CHECK (kind IN ('cut', 'mute', 'scene', 'commercial'));
ALTER TABLE library_streams ADD CONSTRAINT library_streams_kind_check
    CHECK (kind IN ('video', 'audio', 'subtitle'));
ALTER TABLE quality_daily_aggregates ADD CONSTRAINT quality_daily_aggregates_outcome_check
    CHECK (outcome IN ('succeeded', 'empty', 'failed', 'abstained', 'accepted', 'rejected', 'approved',
        'declined', 'playable', 'scheduled'));
ALTER TABLE quality_receipts ADD CONSTRAINT quality_receipts_outcome_check
    CHECK (outcome IN ('succeeded', 'empty', 'failed', 'abstained', 'accepted', 'rejected', 'approved',
        'declined', 'playable', 'scheduled'));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
