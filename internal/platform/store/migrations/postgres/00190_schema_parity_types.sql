-- +goose Up
-- Schema to a 10 (#2793): one schema, two engines. SQLite keeps every integer in 64 bits, so each
-- Postgres twin is bigint; these were 32-bit integer, which holds what is written today but would
-- reject what SQLite accepts. The parity test (schema_parity_postgres_test.go) keeps the engines
-- equal. SQLite needs no change.
ALTER TABLE broadcast_station_seed_revisions ALTER COLUMN version TYPE bigint;
ALTER TABLE channels ALTER COLUMN number TYPE bigint;
ALTER TABLE clips ALTER COLUMN era TYPE bigint;
ALTER TABLE diagnostic_process_runs ALTER COLUMN exit_code TYPE bigint;
ALTER TABLE filler_acquisition_runs
    ALTER COLUMN empty_count TYPE bigint, ALTER COLUMN failed TYPE bigint, ALTER COLUMN fetched TYPE bigint,
    ALTER COLUMN requested TYPE bigint, ALTER COLUMN skipped TYPE bigint;
ALTER TABLE filler_clip_defects
    ALTER COLUMN era TYPE bigint, ALTER COLUMN judge_failures TYPE bigint, ALTER COLUMN label_era TYPE bigint;
ALTER TABLE filler_clip_pipeline
    ALTER COLUMN attempts TYPE bigint, ALTER COLUMN preparation_attempt TYPE bigint,
    ALTER COLUMN preparation_progress TYPE bigint, ALTER COLUMN progress TYPE bigint;
ALTER TABLE filler_enrichment_axes ALTER COLUMN confidence TYPE bigint, ALTER COLUMN evidence_rank TYPE bigint;
ALTER TABLE filler_inference_evaluations ALTER COLUMN attempts TYPE bigint;
ALTER TABLE filler_research_web_usage ALTER COLUMN request_count TYPE bigint;
ALTER TABLE filler_source_check_outcomes ALTER COLUMN item_count TYPE bigint;
ALTER TABLE filler_sources
    ALTER COLUMN check_failure_count TYPE bigint, ALTER COLUMN fetch_every_seconds TYPE bigint,
    ALTER COLUMN fetch_max_per_run TYPE bigint;
ALTER TABLE filler_structure_window_call_ledger ALTER COLUMN attempt TYPE bigint, ALTER COLUMN window_ordinal TYPE bigint;
ALTER TABLE holiday_seed_revisions ALTER COLUMN version TYPE bigint;
ALTER TABLE holidays ALTER COLUMN position TYPE bigint;
ALTER TABLE image_derivatives ALTER COLUMN width TYPE bigint;
ALTER TABLE images
    ALTER COLUMN frame_count TYPE bigint, ALTER COLUMN height TYPE bigint, ALTER COLUMN loop_count TYPE bigint,
    ALTER COLUMN width TYPE bigint;
ALTER TABLE interactive_operations ALTER COLUMN percent TYPE bigint;
ALTER TABLE inventory_source_analysis ALTER COLUMN keyframe_count TYPE bigint, ALTER COLUMN schema_version TYPE bigint;
ALTER TABLE library_art ALTER COLUMN position TYPE bigint, ALTER COLUMN season TYPE bigint;
ALTER TABLE library_candidates ALTER COLUMN rank TYPE bigint, ALTER COLUMN year TYPE bigint;
ALTER TABLE library_catalog ALTER COLUMN version TYPE bigint;
ALTER TABLE library_chapters ALTER COLUMN position TYPE bigint;
ALTER TABLE library_collection_members ALTER COLUMN position TYPE bigint;
ALTER TABLE library_countries ALTER COLUMN position TYPE bigint;
ALTER TABLE library_credits ALTER COLUMN position TYPE bigint;
ALTER TABLE library_filmography ALTER COLUMN position TYPE bigint, ALTER COLUMN year TYPE bigint;
ALTER TABLE library_genres ALTER COLUMN position TYPE bigint;
ALTER TABLE library_items
    ALTER COLUMN episode TYPE bigint, ALTER COLUMN episode_end TYPE bigint, ALTER COLUMN season TYPE bigint,
    ALTER COLUMN year TYPE bigint;
ALTER TABLE library_markers ALTER COLUMN position TYPE bigint;
ALTER TABLE library_streams
    ALTER COLUMN channels TYPE bigint, ALTER COLUMN height TYPE bigint, ALTER COLUMN sample_rate TYPE bigint,
    ALTER COLUMN stream_index TYPE bigint, ALTER COLUMN width TYPE bigint;
ALTER TABLE library_studios ALTER COLUMN position TYPE bigint;
ALTER TABLE library_tags ALTER COLUMN position TYPE bigint;
ALTER TABLE notification_delivery_attempts ALTER COLUMN attempt_number TYPE bigint;
ALTER TABLE proposal_job_attempts ALTER COLUMN attempt TYPE bigint, ALTER COLUMN workflow_version TYPE bigint;
ALTER TABLE proposal_jobs ALTER COLUMN attempts TYPE bigint, ALTER COLUMN workflow_version TYPE bigint;
ALTER TABLE quality_receipts ALTER COLUMN candidate_count TYPE bigint, ALTER COLUMN tool_calls TYPE bigint;
ALTER TABLE quality_run_snapshots ALTER COLUMN schema_version TYPE bigint;
ALTER TABLE secret_protection_metadata ALTER COLUMN singleton TYPE bigint;
ALTER TABLE series_episodes ALTER COLUMN episode_count TYPE bigint;
ALTER TABLE taxonomy_seed_revisions ALTER COLUMN version TYPE bigint;
ALTER TABLE titles
    ALTER COLUMN attempts TYPE bigint, ALTER COLUMN episodes_have TYPE bigint, ALTER COLUMN episodes_wanted TYPE bigint;
ALTER TABLE users ALTER COLUMN quota TYPE bigint;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
