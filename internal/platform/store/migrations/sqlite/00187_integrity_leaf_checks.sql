-- +goose Up
-- Schema to a 9 (#2528), integrity step 1: every foreign key leads an index, and the state columns
-- of tables nothing references accept only the values the code writes (the lint in
-- internal/platform/store/schemadoc/lint.go lists what is left).
-- SQLite cannot add a CHECK to an existing column, so each table is rebuilt transactionally (the
-- 00166 pattern). None of these tables is referenced by another, so a rebuild touches no children.

-- Foreign keys whose parent delete, or child lookup, scanned the whole table.
CREATE INDEX idx_client_crash_reports_user ON client_crash_reports (user_id);
CREATE INDEX idx_device_pairings_user ON device_pairings (user_id);
CREATE INDEX idx_inventory_external_ids_item ON inventory_external_ids (item_id);
CREATE INDEX idx_password_recoveries_user ON password_recoveries (user_id);

-- activity: kind loses its '' default, which the CHECK would refuse; every write names its domain.
DROP INDEX idx_activity_at;
ALTER TABLE activity RENAME TO activity_old;
CREATE TABLE activity (
    id         TEXT   NOT NULL PRIMARY KEY,
    at         BIGINT NOT NULL DEFAULT 0,
    kind       TEXT   NOT NULL CHECK (kind IN ('title', 'channel', 'proposal', 'system')),
    level      TEXT   NOT NULL DEFAULT 'info' CHECK (level IN ('info', 'warn', 'error')),
    text       TEXT   NOT NULL DEFAULT '',
    subject_id TEXT   NOT NULL DEFAULT ''
);
INSERT INTO activity (id, at, kind, level, text, subject_id)
SELECT id, at, kind, level, text, subject_id FROM activity_old;
DROP TABLE activity_old;
CREATE INDEX idx_activity_at ON activity (at DESC);

-- diagnostic_process_runs
DROP INDEX idx_diagnostic_process_runs_time;
DROP INDEX idx_diagnostic_process_runs_status_time;
DROP INDEX idx_diagnostic_process_runs_channel;
DROP INDEX idx_diagnostic_process_runs_job;
DROP INDEX idx_diagnostic_process_runs_parent;
DROP INDEX idx_diagnostic_process_runs_instance;
DROP INDEX idx_diagnostic_process_runs_retention;
ALTER TABLE diagnostic_process_runs RENAME TO diagnostic_process_runs_old;
CREATE TABLE diagnostic_process_runs (
    id                 TEXT   NOT NULL PRIMARY KEY,
    purpose            TEXT   NOT NULL,
    parent_run_id      TEXT   NOT NULL DEFAULT '',
    instance_id        TEXT   NOT NULL DEFAULT '',
    channel_id         TEXT   NOT NULL DEFAULT '',
    target             TEXT   NOT NULL DEFAULT '',
    schedule_block_id  TEXT   NOT NULL DEFAULT '',
    job_id             TEXT   NOT NULL DEFAULT '',
    executable         TEXT   NOT NULL DEFAULT '',
    executable_version TEXT   NOT NULL DEFAULT '',
    command_summary    TEXT   NOT NULL DEFAULT '',
    started_at_ms      BIGINT NOT NULL,
    ended_at_ms        BIGINT NOT NULL DEFAULT 0,
    status             TEXT   NOT NULL
        CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled', 'interrupted')),
    exit_code          INTEGER,
    termination_reason TEXT   NOT NULL DEFAULT '',
    first_error        TEXT   NOT NULL DEFAULT '',
    last_error         TEXT   NOT NULL DEFAULT '',
    output_ref         TEXT   NOT NULL DEFAULT '',
    output_bytes       BIGINT NOT NULL DEFAULT 0,
    discarded_lines    BIGINT NOT NULL DEFAULT 0,
    updated_at_ms      BIGINT NOT NULL,
    size_bytes         BIGINT NOT NULL DEFAULT 0
);
INSERT INTO diagnostic_process_runs (id, purpose, parent_run_id, instance_id, channel_id, target,
    schedule_block_id, job_id, executable, executable_version, command_summary, started_at_ms,
    ended_at_ms, status, exit_code, termination_reason, first_error, last_error, output_ref,
    output_bytes, discarded_lines, updated_at_ms, size_bytes)
SELECT id, purpose, parent_run_id, instance_id, channel_id, target,
    schedule_block_id, job_id, executable, executable_version, command_summary, started_at_ms,
    ended_at_ms, status, exit_code, termination_reason, first_error, last_error, output_ref,
    output_bytes, discarded_lines, updated_at_ms, size_bytes
  FROM diagnostic_process_runs_old;
DROP TABLE diagnostic_process_runs_old;
CREATE INDEX idx_diagnostic_process_runs_time ON diagnostic_process_runs (started_at_ms DESC, id DESC);
CREATE INDEX idx_diagnostic_process_runs_status_time ON diagnostic_process_runs (status, started_at_ms DESC);
CREATE INDEX idx_diagnostic_process_runs_channel ON diagnostic_process_runs (channel_id, started_at_ms DESC);
CREATE INDEX idx_diagnostic_process_runs_job ON diagnostic_process_runs (job_id, started_at_ms DESC);
CREATE INDEX idx_diagnostic_process_runs_parent ON diagnostic_process_runs (parent_run_id, started_at_ms DESC);
CREATE INDEX idx_diagnostic_process_runs_instance ON diagnostic_process_runs (instance_id, started_at_ms DESC);
CREATE INDEX idx_diagnostic_process_runs_retention
    ON diagnostic_process_runs (ended_at_ms, id)
    WHERE status <> 'running' AND ended_at_ms > 0;

-- filler_acquisition_artifacts (and the index its acquisition_id foreign key lacked)
DROP INDEX idx_filler_acquisition_artifacts_path;
DROP INDEX idx_filler_acquisition_artifacts_clip;
DROP INDEX idx_filler_acquisition_artifacts_recovery;
DROP INDEX idx_filler_acquisition_artifacts_remote;
ALTER TABLE filler_acquisition_artifacts RENAME TO filler_acquisition_artifacts_old;
CREATE TABLE filler_acquisition_artifacts (
    id                         TEXT    PRIMARY KEY,
    acquisition_id             TEXT    NOT NULL REFERENCES filler_acquisition_runs(id),
    source_id                  TEXT    NOT NULL DEFAULT '',
    provider                   TEXT    NOT NULL,
    source_url                 TEXT    NOT NULL,
    staging_path               TEXT    NOT NULL DEFAULT '',
    media_path                 TEXT    NOT NULL,
    sidecar_path               TEXT    NOT NULL DEFAULT '',
    media_sha256               TEXT    NOT NULL,
    media_bytes                INTEGER NOT NULL,
    clip_hash                  TEXT    NOT NULL DEFAULT '',
    state                      TEXT    NOT NULL CHECK (state IN ('staged', 'published', 'consumed', 'repair')),
    repair_reason              TEXT    NOT NULL DEFAULT '',
    completed_at               INTEGER NOT NULL,
    updated_at                 INTEGER NOT NULL,
    provider_archive_entry     TEXT    NOT NULL DEFAULT '',
    provider_archive_committed INTEGER NOT NULL DEFAULT 0 CHECK (provider_archive_committed IN (0, 1)),
    remote_id                  TEXT    NOT NULL DEFAULT '',
    gap                        TEXT    NOT NULL DEFAULT ''
);
INSERT INTO filler_acquisition_artifacts (id, acquisition_id, source_id, provider, source_url,
    staging_path, media_path, sidecar_path, media_sha256, media_bytes, clip_hash, state, repair_reason,
    completed_at, updated_at, provider_archive_entry, provider_archive_committed, remote_id, gap)
SELECT id, acquisition_id, source_id, provider, source_url,
    staging_path, media_path, sidecar_path, media_sha256, media_bytes, clip_hash, state, repair_reason,
    completed_at, updated_at, provider_archive_entry, provider_archive_committed, remote_id, gap
  FROM filler_acquisition_artifacts_old;
DROP TABLE filler_acquisition_artifacts_old;
CREATE INDEX idx_filler_acquisition_artifacts_path
    ON filler_acquisition_artifacts (media_path, updated_at DESC);
CREATE INDEX idx_filler_acquisition_artifacts_clip
    ON filler_acquisition_artifacts (clip_hash) WHERE clip_hash <> '';
CREATE INDEX idx_filler_acquisition_artifacts_recovery
    ON filler_acquisition_artifacts (state, updated_at);
CREATE INDEX idx_filler_acquisition_artifacts_remote
    ON filler_acquisition_artifacts (provider, source_id, remote_id) WHERE remote_id <> '';
CREATE INDEX idx_filler_acquisition_artifacts_acquisition
    ON filler_acquisition_artifacts (acquisition_id);

-- filler_clip_defects
DROP INDEX idx_filler_clip_defects_outcome;
DROP INDEX idx_filler_clip_defects_verdict;
ALTER TABLE filler_clip_defects RENAME TO filler_clip_defects_old;
CREATE TABLE filler_clip_defects (
    clip_hash           TEXT    PRIMARY KEY,
    measurement_version TEXT    NOT NULL,
    floor_version       TEXT    NOT NULL,
    era                 INTEGER NOT NULL,
    outcome             TEXT    NOT NULL CHECK (outcome IN ('pass', 'borderline', 'fail', 'undecided')),
    readings_json       TEXT    NOT NULL,
    assessment_json     TEXT    NOT NULL,
    measured_at         INTEGER NOT NULL,
    judgement_json      TEXT    NOT NULL DEFAULT '',
    judge_failures      INTEGER NOT NULL DEFAULT 0,
    verdict             TEXT    NOT NULL DEFAULT '',
    verdict_reason      TEXT    NOT NULL DEFAULT '',
    label               TEXT    NOT NULL DEFAULT '',
    label_era           INTEGER NOT NULL DEFAULT 0,
    labelled_at         INTEGER NOT NULL DEFAULT 0
);
INSERT INTO filler_clip_defects (clip_hash, measurement_version, floor_version, era, outcome,
    readings_json, assessment_json, measured_at, judgement_json, judge_failures, verdict,
    verdict_reason, label, label_era, labelled_at)
SELECT clip_hash, measurement_version, floor_version, era, outcome,
    readings_json, assessment_json, measured_at, judgement_json, judge_failures, verdict,
    verdict_reason, label, label_era, labelled_at
  FROM filler_clip_defects_old;
DROP TABLE filler_clip_defects_old;
CREATE INDEX idx_filler_clip_defects_outcome ON filler_clip_defects (outcome);
CREATE INDEX idx_filler_clip_defects_verdict ON filler_clip_defects (verdict);

-- filler_clip_pipeline
DROP INDEX idx_filler_clip_pipeline_work;
DROP INDEX idx_filler_clip_pipeline_acquisition;
ALTER TABLE filler_clip_pipeline RENAME TO filler_clip_pipeline_old;
CREATE TABLE filler_clip_pipeline (
    clip_hash                TEXT    PRIMARY KEY,
    stage                    TEXT    NOT NULL DEFAULT 'probe',
    -- ⚠ failed retries with backoff, skipped is re-evaluated every pass but never re-run, done is
    -- never redone: three meanings the vocabulary exists to keep apart.
    status                   TEXT    NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'done', 'failed', 'skipped')),
    progress                 INTEGER NOT NULL DEFAULT 0,
    disposition              TEXT    NOT NULL DEFAULT 'running',
    reject_reason            TEXT    NOT NULL DEFAULT '',
    reject_detail            TEXT    NOT NULL DEFAULT '',
    attempts                 INTEGER NOT NULL DEFAULT 0,
    next_run                 INTEGER NOT NULL DEFAULT 0,
    stages_json              TEXT    NOT NULL DEFAULT '[]',
    enrolled_at              INTEGER NOT NULL,
    updated_at               INTEGER NOT NULL,
    force_run                INTEGER NOT NULL DEFAULT 0,
    acquisition_id           TEXT    NOT NULL DEFAULT '',
    preparation_attempt      INTEGER NOT NULL DEFAULT 0,
    preparation_started_at   INTEGER NOT NULL DEFAULT 0,
    preparation_start_reason TEXT    NOT NULL DEFAULT '',
    preparation_progress     INTEGER NOT NULL DEFAULT -1,
    stage_queued_at          INTEGER NOT NULL DEFAULT 0,
    stage_started_at         INTEGER NOT NULL DEFAULT 0
);
INSERT INTO filler_clip_pipeline (clip_hash, stage, status, progress, disposition, reject_reason,
    reject_detail, attempts, next_run, stages_json, enrolled_at, updated_at, force_run,
    acquisition_id, preparation_attempt, preparation_started_at, preparation_start_reason,
    preparation_progress, stage_queued_at, stage_started_at)
SELECT clip_hash, stage, status, progress, disposition, reject_reason,
    reject_detail, attempts, next_run, stages_json, enrolled_at, updated_at, force_run,
    acquisition_id, preparation_attempt, preparation_started_at, preparation_start_reason,
    preparation_progress, stage_queued_at, stage_started_at
  FROM filler_clip_pipeline_old;
DROP TABLE filler_clip_pipeline_old;
CREATE INDEX idx_filler_clip_pipeline_work ON filler_clip_pipeline (disposition, next_run);
CREATE INDEX idx_filler_clip_pipeline_acquisition ON filler_clip_pipeline (acquisition_id);

-- filler_curated_offers ('' means the household hasn't touched the offered source)
ALTER TABLE filler_curated_offers RENAME TO filler_curated_offers_old;
CREATE TABLE filler_curated_offers (
    source_id    TEXT    PRIMARY KEY,
    list_version TEXT    NOT NULL,
    offered_at   INTEGER NOT NULL,
    stations     TEXT    NOT NULL DEFAULT '',
    local_state  TEXT    NOT NULL DEFAULT '' CHECK (local_state IN ('', 'on', 'operator', 'off'))
);
INSERT INTO filler_curated_offers (source_id, list_version, offered_at, stations, local_state)
SELECT source_id, list_version, offered_at, stations, local_state FROM filler_curated_offers_old;
DROP TABLE filler_curated_offers_old;

-- filler_enrichment_axes (a missing axis is never stored)
DROP INDEX idx_filler_enrichment_axes_state_axis;
ALTER TABLE filler_enrichment_axes RENAME TO filler_enrichment_axes_old;
CREATE TABLE filler_enrichment_axes (
    clip_hash          TEXT    NOT NULL,
    axis               TEXT    NOT NULL,
    state              TEXT    NOT NULL CHECK (state IN ('complete', 'unsupported', 'stale')),
    value_json         TEXT    NOT NULL DEFAULT '{}',
    evidence_kind      TEXT    NOT NULL,
    evidence_rank      INTEGER NOT NULL,
    evidence_reference TEXT    NOT NULL,
    confidence         INTEGER NOT NULL,
    producer           TEXT    NOT NULL,
    producer_version   TEXT    NOT NULL,
    taxonomy_version   TEXT    NOT NULL DEFAULT '',
    observed_at        INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL,
    PRIMARY KEY (clip_hash, axis)
);
INSERT INTO filler_enrichment_axes (clip_hash, axis, state, value_json, evidence_kind, evidence_rank,
    evidence_reference, confidence, producer, producer_version, taxonomy_version, observed_at, updated_at)
SELECT clip_hash, axis, state, value_json, evidence_kind, evidence_rank,
    evidence_reference, confidence, producer, producer_version, taxonomy_version, observed_at, updated_at
  FROM filler_enrichment_axes_old;
DROP TABLE filler_enrichment_axes_old;
CREATE INDEX idx_filler_enrichment_axes_state_axis ON filler_enrichment_axes (state, axis);

-- filler_ready_events: outcome only ever held 'ready' (every row is a clip becoming ready), so it goes
DROP INDEX idx_filler_ready_events_recent;
DROP INDEX idx_filler_ready_events_clip;
ALTER TABLE filler_ready_events RENAME TO filler_ready_events_old;
CREATE TABLE filler_ready_events (
    id              TEXT    PRIMARY KEY,
    clip_hash       TEXT    NOT NULL,
    acquisition_id  TEXT    NOT NULL DEFAULT '',
    enrollment_kind TEXT    NOT NULL,
    enrollment_ref  TEXT    NOT NULL,
    placement       TEXT    NOT NULL,
    created_at      INTEGER NOT NULL
);
INSERT INTO filler_ready_events (id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref,
    placement, created_at)
SELECT id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref,
    placement, created_at
  FROM filler_ready_events_old;
DROP TABLE filler_ready_events_old;
CREATE INDEX idx_filler_ready_events_recent ON filler_ready_events (created_at DESC, id DESC);
CREATE UNIQUE INDEX idx_filler_ready_events_clip ON filler_ready_events (clip_hash);

-- interactive_operations
ALTER TABLE interactive_operations RENAME TO interactive_operations_old;
CREATE TABLE interactive_operations (
    id           TEXT    PRIMARY KEY,
    kind         TEXT    NOT NULL CHECK (kind IN ('filler_split', 'llm_pull')),
    subject      TEXT    NOT NULL,
    status       TEXT    NOT NULL CHECK (status IN ('queued', 'running', 'success', 'error')),
    percent      INTEGER NOT NULL DEFAULT 0,
    completed    BIGINT  NOT NULL DEFAULT 0,
    total        BIGINT  NOT NULL DEFAULT 0,
    result_id    TEXT    NOT NULL DEFAULT '',
    error        TEXT    NOT NULL DEFAULT '',
    started_at   BIGINT  NOT NULL,
    completed_at BIGINT  NOT NULL DEFAULT 0,
    updated_at   BIGINT  NOT NULL
);
INSERT INTO interactive_operations (id, kind, subject, status, percent, completed, total, result_id,
    error, started_at, completed_at, updated_at)
SELECT id, kind, subject, status, percent, completed, total, result_id,
    error, started_at, completed_at, updated_at
  FROM interactive_operations_old;
DROP TABLE interactive_operations_old;

-- library_answers
ALTER TABLE library_answers RENAME TO library_answers_old;
CREATE TABLE library_answers (
    origin_authority TEXT    NOT NULL,
    origin_key       TEXT    NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    tmdb_id          TEXT    NOT NULL DEFAULT '',
    ignored          INTEGER NOT NULL DEFAULT 0,
    answered_at_ms   INTEGER NOT NULL,
    answered_by      TEXT    NOT NULL DEFAULT 'you',
    PRIMARY KEY (origin_authority, origin_key)
);
INSERT INTO library_answers (origin_authority, origin_key, kind, tmdb_id, ignored, answered_at_ms, answered_by)
SELECT origin_authority, origin_key, kind, tmdb_id, ignored, answered_at_ms, answered_by
  FROM library_answers_old;
DROP TABLE library_answers_old;

-- library_candidates
ALTER TABLE library_candidates RENAME TO library_candidates_old;
CREATE TABLE library_candidates (
    inventory_item_id TEXT    NOT NULL,
    rank              INTEGER NOT NULL,
    kind              TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    tmdb_id           TEXT    NOT NULL,
    name              TEXT    NOT NULL,
    year              INTEGER NOT NULL DEFAULT 0,
    overview          TEXT    NOT NULL DEFAULT '',
    poster            TEXT    NOT NULL DEFAULT '',
    name_match        INTEGER NOT NULL DEFAULT 0,
    year_match        INTEGER NOT NULL DEFAULT 0,
    length_match      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (inventory_item_id, rank)
);
INSERT INTO library_candidates (inventory_item_id, rank, kind, tmdb_id, name, year, overview, poster,
    name_match, year_match, length_match)
SELECT inventory_item_id, rank, kind, tmdb_id, name, year, overview, poster,
    name_match, year_match, length_match
  FROM library_candidates_old;
DROP TABLE library_candidates_old;

-- library_catalog
DROP INDEX idx_library_catalog_fetched;
ALTER TABLE library_catalog RENAME TO library_catalog_old;
CREATE TABLE library_catalog (
    provider      TEXT    NOT NULL,
    kind          TEXT    NOT NULL CHECK (kind IN ('movie', 'series', 'episode', 'season', 'collection')),
    external_id   TEXT    NOT NULL,
    record        TEXT    NOT NULL DEFAULT '',
    fetched_at_ms INTEGER NOT NULL DEFAULT 0,
    retry_at_ms   INTEGER NOT NULL DEFAULT 0,
    version       INTEGER NOT NULL DEFAULT 0,
    language      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (provider, kind, external_id)
);
INSERT INTO library_catalog (provider, kind, external_id, record, fetched_at_ms, retry_at_ms, version, language)
SELECT provider, kind, external_id, record, fetched_at_ms, retry_at_ms, version, language
  FROM library_catalog_old;
DROP TABLE library_catalog_old;
CREATE INDEX idx_library_catalog_fetched ON library_catalog (fetched_at_ms);

-- library_files ('' is a problem file the scan couldn't place)
DROP INDEX idx_library_files_root;
DROP INDEX idx_library_files_inventory_item;
DROP INDEX idx_library_files_inventory_series;
ALTER TABLE library_files RENAME TO library_files_old;
CREATE TABLE library_files (
    path                TEXT    PRIMARY KEY,
    root                TEXT    NOT NULL,
    size_bytes          INTEGER NOT NULL,
    modified_ns         INTEGER NOT NULL,
    identity            TEXT    NOT NULL DEFAULT '',
    kind                TEXT    NOT NULL DEFAULT '' CHECK (kind IN ('', 'movie', 'episode', 'series')),
    header              TEXT    NOT NULL DEFAULT 'pending',
    problem             TEXT    NOT NULL DEFAULT '',
    held                INTEGER NOT NULL DEFAULT 0,
    seen_at_ms          INTEGER NOT NULL,
    missing_at_ms       INTEGER NOT NULL DEFAULT 0,
    inventory_item_id   TEXT    NOT NULL DEFAULT '',
    inventory_series_id TEXT    NOT NULL DEFAULT ''
);
INSERT INTO library_files (path, root, size_bytes, modified_ns, identity, kind, header, problem, held,
    seen_at_ms, missing_at_ms, inventory_item_id, inventory_series_id)
SELECT path, root, size_bytes, modified_ns, identity, kind, header, problem, held,
    seen_at_ms, missing_at_ms, inventory_item_id, inventory_series_id
  FROM library_files_old
 -- An `extra` row is a file a scan from before #2151 decision 60 recorded; the scan skips extras
 -- now and writes no row, so the row goes rather than failing the CHECK.
 WHERE kind IN ('', 'movie', 'episode', 'series');
DROP TABLE library_files_old;
CREATE INDEX idx_library_files_root ON library_files (root);
CREATE INDEX idx_library_files_inventory_item ON library_files (inventory_item_id);
CREATE INDEX idx_library_files_inventory_series ON library_files (inventory_series_id);

-- library_filmography
DROP INDEX idx_library_filmography_title;
ALTER TABLE library_filmography RENAME TO library_filmography_old;
CREATE TABLE library_filmography (
    person_id      TEXT    NOT NULL REFERENCES library_people(id) ON DELETE CASCADE,
    position       INTEGER NOT NULL,
    kind           TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    tmdb_id        TEXT    NOT NULL,
    title          TEXT    NOT NULL,
    year           INTEGER NOT NULL DEFAULT 0,
    role           TEXT    NOT NULL,
    character_name TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (person_id, position)
);
INSERT INTO library_filmography (person_id, position, kind, tmdb_id, title, year, role, character_name)
SELECT person_id, position, kind, tmdb_id, title, year, role, character_name
  FROM library_filmography_old;
DROP TABLE library_filmography_old;
CREATE INDEX idx_library_filmography_title ON library_filmography (kind, tmdb_id);

-- library_id_map
ALTER TABLE library_id_map RENAME TO library_id_map_old;
CREATE TABLE library_id_map (
    media_id     TEXT    PRIMARY KEY,
    kind         TEXT    NOT NULL CHECK (kind IN ('movie', 'series', 'episode')),
    item_id      TEXT    NOT NULL DEFAULT '',
    reason       TEXT    NOT NULL DEFAULT '',
    mapped_at_ms INTEGER NOT NULL
);
INSERT INTO library_id_map (media_id, kind, item_id, reason, mapped_at_ms)
SELECT media_id, kind, item_id, reason, mapped_at_ms FROM library_id_map_old;
DROP TABLE library_id_map_old;

-- library_markers
ALTER TABLE library_markers RENAME TO library_markers_old;
CREATE TABLE library_markers (
    item_id  TEXT    NOT NULL,
    path     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms   INTEGER NOT NULL,
    kind     TEXT    NOT NULL CHECK (kind IN ('cut', 'mute', 'scene', 'commercial')),
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
INSERT INTO library_markers (item_id, path, position, start_ms, end_ms, kind)
SELECT item_id, path, position, start_ms, end_ms, kind FROM library_markers_old;
DROP TABLE library_markers_old;

-- library_streams
DROP INDEX idx_library_streams_language;
ALTER TABLE library_streams RENAME TO library_streams_old;
CREATE TABLE library_streams (
    item_id          TEXT    NOT NULL,
    path             TEXT    NOT NULL,
    stream_index     INTEGER NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('video', 'audio', 'subtitle')),
    codec            TEXT    NOT NULL DEFAULT '',
    profile          TEXT    NOT NULL DEFAULT '',
    title            TEXT    NOT NULL DEFAULT '',
    is_default       BOOLEAN NOT NULL DEFAULT FALSE,
    forced           BOOLEAN NOT NULL DEFAULT FALSE,
    language         TEXT    NOT NULL DEFAULT '',
    language_name    TEXT    NOT NULL DEFAULT '',
    channels         INTEGER NOT NULL DEFAULT 0,
    channel_layout   TEXT    NOT NULL DEFAULT '',
    sample_rate      INTEGER NOT NULL DEFAULT 0,
    audio_format     TEXT    NOT NULL DEFAULT '',
    width            INTEGER NOT NULL DEFAULT 0,
    height           INTEGER NOT NULL DEFAULT 0,
    frame_rate       TEXT    NOT NULL DEFAULT '',
    pixel_format     TEXT    NOT NULL DEFAULT '',
    color_transfer   TEXT    NOT NULL DEFAULT '',
    resolution       TEXT    NOT NULL DEFAULT '',
    hdr              TEXT    NOT NULL DEFAULT '',
    commentary       BOOLEAN NOT NULL DEFAULT FALSE,
    hearing_impaired BOOLEAN NOT NULL DEFAULT FALSE,
    descriptive      BOOLEAN NOT NULL DEFAULT FALSE,
    external_ref     TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, stream_index),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
INSERT INTO library_streams (item_id, path, stream_index, kind, codec, profile, title, is_default,
    forced, language, language_name, channels, channel_layout, sample_rate, audio_format, width,
    height, frame_rate, pixel_format, color_transfer, resolution, hdr, commentary, hearing_impaired,
    descriptive, external_ref)
SELECT item_id, path, stream_index, kind, codec, profile, title, is_default,
    forced, language, language_name, channels, channel_layout, sample_rate, audio_format, width,
    height, frame_rate, pixel_format, color_transfer, resolution, hdr, commentary, hearing_impaired,
    descriptive, external_ref
  FROM library_streams_old;
DROP TABLE library_streams_old;
CREATE INDEX idx_library_streams_language ON library_streams (kind, language);

-- quality_daily_aggregates
ALTER TABLE quality_daily_aggregates RENAME TO quality_daily_aggregates_old;
CREATE TABLE quality_daily_aggregates (
    day               TEXT   NOT NULL,
    stage             TEXT   NOT NULL,
    outcome           TEXT   NOT NULL CHECK (outcome IN ('succeeded', 'empty', 'failed', 'abstained',
        'accepted', 'rejected', 'approved', 'declined', 'playable', 'scheduled')),
    run_snapshot_id   TEXT   NOT NULL DEFAULT '',
    observation_count BIGINT NOT NULL,
    duration_millis   BIGINT NOT NULL,
    tool_calls        BIGINT NOT NULL,
    candidate_count   BIGINT NOT NULL,
    cost_nanos        BIGINT NOT NULL,
    PRIMARY KEY (day, stage, outcome, run_snapshot_id)
);
INSERT INTO quality_daily_aggregates (day, stage, outcome, run_snapshot_id, observation_count,
    duration_millis, tool_calls, candidate_count, cost_nanos)
SELECT day, stage, outcome, run_snapshot_id, observation_count,
    duration_millis, tool_calls, candidate_count, cost_nanos
  FROM quality_daily_aggregates_old;
DROP TABLE quality_daily_aggregates_old;

-- quality_receipts (and the index its run_snapshot_id foreign key lacked)
DROP INDEX idx_quality_receipts_rollup;
ALTER TABLE quality_receipts RENAME TO quality_receipts_old;
CREATE TABLE quality_receipts (
    idempotency_key TEXT    PRIMARY KEY,
    occurred_at     BIGINT  NOT NULL,
    stage           TEXT    NOT NULL,
    outcome         TEXT    NOT NULL CHECK (outcome IN ('succeeded', 'empty', 'failed', 'abstained',
        'accepted', 'rejected', 'approved', 'declined', 'playable', 'scheduled')),
    duration_millis BIGINT  NOT NULL,
    tool_calls      INTEGER NOT NULL,
    candidate_count INTEGER NOT NULL,
    cost_nanos      BIGINT  NOT NULL,
    run_snapshot_id TEXT    REFERENCES quality_run_snapshots(id)
);
INSERT INTO quality_receipts (idempotency_key, occurred_at, stage, outcome, duration_millis, tool_calls,
    candidate_count, cost_nanos, run_snapshot_id)
SELECT idempotency_key, occurred_at, stage, outcome, duration_millis, tool_calls,
    candidate_count, cost_nanos, run_snapshot_id
  FROM quality_receipts_old;
DROP TABLE quality_receipts_old;
CREATE INDEX idx_quality_receipts_rollup ON quality_receipts (occurred_at, stage, outcome);
CREATE INDEX idx_quality_receipts_run_snapshot ON quality_receipts (run_snapshot_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
