-- +goose Up
-- Schema to a 10 (#2793): every JSON column is declared one way on both engines, TEXT with
-- CHECK (json_valid(col)) in SQLite and jsonb in Postgres (the Postgres twin). Five columns used ''
-- for "no document", which is not JSON: channels.intent_json, filler_clip_defects.judgement_json,
-- filler_structure_window_call_ledger.record_json, proposal_jobs.failure_trace_json and
-- holidays.air_window. They become nullable, and the copy turns each '' into NULL. The ledger's
-- settled/unsettled CHECK tests record_json for NULL instead of ''.
-- SQLite cannot add a CHECK to an existing column, so each table is rebuilt with its descendants
-- (the 00188 pattern): children are renamed first, created after the parent, and dropped first.
-- Copies keep their columns in order, so INSERT ... SELECT * carries every value across, except
-- where a column turns '' into NULL.

-- channels and its children ---------------------------------------------------------------------
ALTER TABLE airings RENAME TO airings_old;
ALTER TABLE scheduled_airings RENAME TO scheduled_airings_old;
ALTER TABLE user_favourite_channels RENAME TO user_favourite_channels_old;
ALTER TABLE user_recent_channels RENAME TO user_recent_channels_old;
ALTER TABLE channels RENAME TO channels_old;
CREATE TABLE channels (
    id                 TEXT    PRIMARY KEY,
    intent_ref         TEXT    NOT NULL DEFAULT '',
    name               TEXT    NOT NULL,
    number             INTEGER NOT NULL,
    grp                TEXT    NOT NULL DEFAULT '',   -- the channel's group ("group" is reserved)
    logo               TEXT    NOT NULL DEFAULT '',
    strategy           TEXT    NOT NULL,
    tunarr_id          TEXT    NOT NULL DEFAULT '',
    status             TEXT    NOT NULL
        CHECK (status IN ('building', 'live', 'drifted', 'empty', 'detached', 'paused')),
    shuffle_seed       BIGINT  NOT NULL DEFAULT 0,
    lineup_json        TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(lineup_json)),
    desired_json       TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(desired_json)),
    -- reconcile_deadline drives ClaimDueChannels: a channel is due when its deadline is at or
    -- before now; claiming leases it so two replicas don't reconcile one channel at once.
    reconcile_deadline BIGINT  NOT NULL DEFAULT 0,
    updated_at         BIGINT  NOT NULL DEFAULT 0,
    policy_json        TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(policy_json)),
    broadcast_codec    TEXT    NOT NULL DEFAULT 'h264',
    revision           BIGINT  NOT NULL DEFAULT 1 CHECK (revision >= 1),
    playout_anchor     BIGINT  NOT NULL DEFAULT 0 CHECK (playout_anchor >= 0),
    created_at         BIGINT  NOT NULL DEFAULT 0,
    intent_json        TEXT    CHECK (json_valid(intent_json)),
    requested_by       TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
CREATE TABLE scheduled_airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
CREATE TABLE user_favourite_channels (
    user_id     TEXT   NOT NULL,
    channel_id  TEXT   NOT NULL,
    added_at_ms BIGINT NOT NULL CHECK (added_at_ms > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
CREATE TABLE user_recent_channels (
    user_id     TEXT   NOT NULL,
    channel_id  TEXT   NOT NULL,
    tuned_at_ms BIGINT NOT NULL CHECK (tuned_at_ms > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
INSERT INTO channels (id, intent_ref, name, number, grp, logo, strategy, tunarr_id, status, shuffle_seed,
    lineup_json, desired_json, reconcile_deadline, updated_at, policy_json, broadcast_codec, revision,
    playout_anchor, created_at, intent_json, requested_by)
SELECT id, intent_ref, name, number, grp, logo, strategy, tunarr_id, status, shuffle_seed,
    lineup_json, desired_json, reconcile_deadline, updated_at, policy_json, broadcast_codec, revision,
    playout_anchor, created_at, NULLIF(intent_json, ''), requested_by
  FROM channels_old;
INSERT INTO airings SELECT * FROM airings_old;
INSERT INTO scheduled_airings SELECT * FROM scheduled_airings_old;
INSERT INTO user_favourite_channels SELECT * FROM user_favourite_channels_old;
INSERT INTO user_recent_channels SELECT * FROM user_recent_channels_old;
DROP TABLE airings_old;
DROP TABLE scheduled_airings_old;
DROP TABLE user_favourite_channels_old;
DROP TABLE user_recent_channels_old;
DROP TABLE channels_old;
CREATE UNIQUE INDEX idx_channels_number ON channels (number);
CREATE INDEX idx_channels_status_deadline ON channels (status, reconcile_deadline);
CREATE UNIQUE INDEX idx_channels_intent_ref ON channels (intent_ref) WHERE intent_ref <> '';
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);
CREATE INDEX idx_scheduled_airings_channel ON scheduled_airings (channel_id, recorded_at);
CREATE INDEX idx_user_favourite_channels_channel ON user_favourite_channels (channel_id);
CREATE INDEX idx_user_recent_channels_channel ON user_recent_channels (channel_id);
CREATE INDEX idx_user_recent_channels_user_tuned ON user_recent_channels (user_id, tuned_at_ms DESC);

-- client_crash_reports, diagnostic_events ------------------------------------------------------
ALTER TABLE client_crash_reports RENAME TO client_crash_reports_old;
ALTER TABLE diagnostic_events RENAME TO diagnostic_events_old;
CREATE TABLE client_crash_reports (
    id               TEXT PRIMARY KEY,
    received_at_ms   BIGINT NOT NULL CHECK (received_at_ms > 0),
    occurred_at_ms   BIGINT NOT NULL,
    user_id          TEXT REFERENCES users(id) ON DELETE SET NULL,
    device_id        TEXT NOT NULL DEFAULT '',
    device_name      TEXT NOT NULL DEFAULT '',
    kind             TEXT NOT NULL CHECK (kind IN ('js-fatal', 'js-error', 'native')),
    platform         TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'tvos', 'android-tv')),
    app_version      TEXT NOT NULL,
    app_build        TEXT NOT NULL,
    os_version       TEXT NOT NULL,
    device_model     TEXT NOT NULL,
    message          TEXT NOT NULL,
    stack            TEXT NOT NULL DEFAULT '',
    breadcrumbs_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(breadcrumbs_json)),
    native_payload   TEXT NOT NULL DEFAULT ''
);
CREATE TABLE diagnostic_events (
    id                  TEXT   NOT NULL PRIMARY KEY,
    occurred_at_ms      BIGINT NOT NULL,
    received_at_ms      BIGINT NOT NULL,
    level               TEXT   NOT NULL,
    source              TEXT   NOT NULL,
    subsystem           TEXT   NOT NULL DEFAULT '',
    event               TEXT   NOT NULL,
    message             TEXT   NOT NULL DEFAULT '',
    request_id          TEXT   NOT NULL DEFAULT '',
    playback_session_id TEXT   NOT NULL DEFAULT '',
    channel_id          TEXT   NOT NULL DEFAULT '',
    schedule_block_id   TEXT   NOT NULL DEFAULT '',
    job_id              TEXT   NOT NULL DEFAULT '',
    process_run_id      TEXT   NOT NULL DEFAULT '',
    actor_id            TEXT   NOT NULL DEFAULT '',
    instance_id         TEXT   NOT NULL DEFAULT '',
    attributes_json     TEXT   NOT NULL DEFAULT '{}' CHECK (json_valid(attributes_json)),
    size_bytes          BIGINT NOT NULL DEFAULT 0
);
INSERT INTO client_crash_reports SELECT * FROM client_crash_reports_old;
INSERT INTO diagnostic_events SELECT * FROM diagnostic_events_old;
DROP TABLE client_crash_reports_old;
DROP TABLE diagnostic_events_old;
CREATE INDEX idx_client_crash_reports_received ON client_crash_reports (received_at_ms, id);
CREATE INDEX idx_client_crash_reports_user ON client_crash_reports (user_id);
CREATE INDEX idx_diagnostic_events_time ON diagnostic_events (occurred_at_ms DESC, id DESC);
CREATE INDEX idx_diagnostic_events_level_time ON diagnostic_events (level, occurred_at_ms DESC);
CREATE INDEX idx_diagnostic_events_subsystem_time ON diagnostic_events (subsystem, occurred_at_ms DESC);
CREATE INDEX idx_diagnostic_events_request ON diagnostic_events (request_id, occurred_at_ms DESC) WHERE request_id <> '';
CREATE INDEX idx_diagnostic_events_playback ON diagnostic_events (playback_session_id, occurred_at_ms DESC) WHERE playback_session_id <> '';
CREATE INDEX idx_diagnostic_events_channel ON diagnostic_events (channel_id, occurred_at_ms DESC) WHERE channel_id <> '';
CREATE INDEX idx_diagnostic_events_schedule_block ON diagnostic_events (schedule_block_id, occurred_at_ms DESC) WHERE schedule_block_id <> '';
CREATE INDEX idx_diagnostic_events_job ON diagnostic_events (job_id, occurred_at_ms DESC) WHERE job_id <> '';
CREATE INDEX idx_diagnostic_events_process ON diagnostic_events (process_run_id, occurred_at_ms DESC) WHERE process_run_id <> '';

-- the filler pipeline's documents --------------------------------------------------------------
ALTER TABLE filler_clip_defects RENAME TO filler_clip_defects_old;
ALTER TABLE filler_clip_fingerprints RENAME TO filler_clip_fingerprints_old;
ALTER TABLE filler_clip_pipeline RENAME TO filler_clip_pipeline_old;
ALTER TABLE filler_context_reports RENAME TO filler_context_reports_old;
ALTER TABLE filler_enrichment_axes RENAME TO filler_enrichment_axes_old;
ALTER TABLE filler_split_proposals RENAME TO filler_split_proposals_old;
ALTER TABLE filler_split_shadow_decisions RENAME TO filler_split_shadow_decisions_old;
CREATE TABLE filler_clip_defects (
    clip_hash           TEXT    PRIMARY KEY,
    measurement_version TEXT    NOT NULL,
    floor_version       TEXT    NOT NULL,
    era                 INTEGER NOT NULL,
    outcome             TEXT    NOT NULL CHECK (outcome IN ('pass', 'borderline', 'fail', 'undecided')),
    readings_json       TEXT    NOT NULL CHECK (json_valid(readings_json)),
    assessment_json     TEXT    NOT NULL CHECK (json_valid(assessment_json)),
    measured_at         INTEGER NOT NULL,
    judgement_json      TEXT    CHECK (json_valid(judgement_json)),
    judge_failures      INTEGER NOT NULL DEFAULT 0,
    verdict             TEXT    NOT NULL DEFAULT '',
    verdict_reason      TEXT    NOT NULL DEFAULT '',
    label               TEXT    NOT NULL DEFAULT '',
    label_era           INTEGER NOT NULL DEFAULT 0,
    labelled_at         INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE filler_clip_fingerprints (
    clip_hash        TEXT NOT NULL,
    algorithm        TEXT NOT NULL,
    fingerprint_json TEXT NOT NULL CHECK (json_valid(fingerprint_json)),
    PRIMARY KEY (clip_hash, algorithm)
);
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
    stages_json              TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(stages_json)),
    enrolled_at              INTEGER NOT NULL,
    updated_at               INTEGER NOT NULL,
    force_run                BOOLEAN NOT NULL DEFAULT FALSE CHECK (force_run IN (0, 1)),
    acquisition_id           TEXT    NOT NULL DEFAULT '',
    preparation_attempt      INTEGER NOT NULL DEFAULT 0,
    preparation_started_at   INTEGER NOT NULL DEFAULT 0,
    preparation_start_reason TEXT    NOT NULL DEFAULT '',
    preparation_progress     INTEGER NOT NULL DEFAULT -1,
    stage_queued_at          INTEGER NOT NULL DEFAULT 0,
    stage_started_at         INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE filler_context_reports (
    clip_hash        TEXT    NOT NULL,
    producer         TEXT    NOT NULL,
    producer_version TEXT    NOT NULL,
    adapter          TEXT    NOT NULL,
    adapter_version  TEXT    NOT NULL,
    input_revision   INTEGER NOT NULL,
    report_json      TEXT    NOT NULL CHECK (json_valid(report_json)),
    completed_at     INTEGER NOT NULL,
    PRIMARY KEY (clip_hash, producer, producer_version, adapter, adapter_version, input_revision)
);
CREATE TABLE filler_enrichment_axes (
    clip_hash          TEXT    NOT NULL,
    axis               TEXT    NOT NULL,
    state              TEXT    NOT NULL CHECK (state IN ('complete', 'unsupported', 'stale')),
    value_json         TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(value_json)),
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
CREATE TABLE filler_split_proposals (
    id               TEXT    PRIMARY KEY,
    clip_hash        TEXT    NOT NULL UNIQUE,  -- the compilation's clip identity (path rel. FILLER_DIR)
    segments_json    TEXT    NOT NULL CHECK (json_valid(segments_json)), -- []filler.SplitSegment
    created_at       INTEGER NOT NULL,
    claim_token      TEXT    NOT NULL DEFAULT '',
    claim_expires_at INTEGER NOT NULL DEFAULT 0,
    CHECK ((claim_token = '' AND claim_expires_at = 0) OR
           (claim_token <> '' AND claim_expires_at > 0))
);
CREATE TABLE filler_split_shadow_decisions (
    id                TEXT    PRIMARY KEY,
    proposal_id       TEXT    NOT NULL,
    clip_hash         TEXT    NOT NULL,
    source_sha256     TEXT    NOT NULL DEFAULT '',
    assessment_sha256 TEXT    NOT NULL DEFAULT '',
    policy_version    TEXT    NOT NULL,
    decision_json     TEXT    NOT NULL CHECK (json_valid(decision_json)),
    observed_at       INTEGER NOT NULL
);
INSERT INTO filler_clip_defects (clip_hash, measurement_version, floor_version, era, outcome, readings_json,
    assessment_json, measured_at, judgement_json, judge_failures, verdict, verdict_reason, label, label_era,
    labelled_at)
SELECT clip_hash, measurement_version, floor_version, era, outcome, readings_json,
    assessment_json, measured_at, NULLIF(judgement_json, ''), judge_failures, verdict, verdict_reason, label,
    label_era, labelled_at
  FROM filler_clip_defects_old;
INSERT INTO filler_clip_fingerprints SELECT * FROM filler_clip_fingerprints_old;
INSERT INTO filler_clip_pipeline SELECT * FROM filler_clip_pipeline_old;
INSERT INTO filler_context_reports SELECT * FROM filler_context_reports_old;
INSERT INTO filler_enrichment_axes SELECT * FROM filler_enrichment_axes_old;
INSERT INTO filler_split_proposals SELECT * FROM filler_split_proposals_old;
INSERT INTO filler_split_shadow_decisions SELECT * FROM filler_split_shadow_decisions_old;
DROP TABLE filler_clip_defects_old;
DROP TABLE filler_clip_fingerprints_old;
DROP TABLE filler_clip_pipeline_old;
DROP TABLE filler_context_reports_old;
DROP TABLE filler_enrichment_axes_old;
DROP TABLE filler_split_proposals_old;
DROP TABLE filler_split_shadow_decisions_old;
CREATE INDEX idx_filler_clip_defects_outcome ON filler_clip_defects (outcome);
CREATE INDEX idx_filler_clip_defects_verdict ON filler_clip_defects (verdict);
CREATE INDEX idx_filler_clip_fingerprints_algorithm ON filler_clip_fingerprints (algorithm);
CREATE INDEX idx_filler_clip_pipeline_work ON filler_clip_pipeline (disposition, next_run);
CREATE INDEX idx_filler_clip_pipeline_acquisition ON filler_clip_pipeline (acquisition_id);
CREATE INDEX idx_filler_context_reports_latest ON filler_context_reports (clip_hash, completed_at DESC);
CREATE INDEX idx_filler_enrichment_axes_state_axis ON filler_enrichment_axes (state, axis);
CREATE INDEX idx_filler_split_shadow_decisions_clip
    ON filler_split_shadow_decisions (clip_hash, observed_at DESC, id DESC);
CREATE INDEX idx_filler_split_shadow_decisions_proposal
    ON filler_split_shadow_decisions (proposal_id, observed_at DESC, id DESC);

-- filler_inference_evaluations and its ledger --------------------------------------------------
ALTER TABLE filler_structure_window_call_ledger RENAME TO filler_structure_window_call_ledger_old;
ALTER TABLE filler_inference_evaluations RENAME TO filler_inference_evaluations_old;
CREATE TABLE filler_inference_evaluations (
    id                       TEXT    PRIMARY KEY,
    cache_key                TEXT    NOT NULL,
    clip_hash                TEXT    NOT NULL,
    run_id                   TEXT    NOT NULL DEFAULT '',
    role                     TEXT    NOT NULL,
    rung                     TEXT    NOT NULL DEFAULT '',
    state                    TEXT    NOT NULL CHECK (state IN ('reserved', 'completed', 'failed', 'held_budget')),
    requested_provider       TEXT    NOT NULL,
    requested_model          TEXT    NOT NULL,
    resolved_provider        TEXT    NOT NULL DEFAULT '',
    resolved_model           TEXT    NOT NULL DEFAULT '',
    upstream_provider        TEXT    NOT NULL DEFAULT '',
    modalities_json          TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(modalities_json)),
    derivative_bytes         INTEGER NOT NULL DEFAULT 0,
    derivative_duration_ms   INTEGER NOT NULL DEFAULT 0,
    derivative_pixels        INTEGER NOT NULL DEFAULT 0,
    prompt_tokens            INTEGER NOT NULL DEFAULT 0,
    completion_tokens        INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens         INTEGER NOT NULL DEFAULT 0,
    cached_tokens            INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens       INTEGER NOT NULL DEFAULT 0,
    image_tokens             INTEGER NOT NULL DEFAULT 0,
    audio_tokens             INTEGER NOT NULL DEFAULT 0,
    video_tokens             INTEGER NOT NULL DEFAULT 0,
    charged_amount           TEXT    NOT NULL DEFAULT '',
    charged_currency         TEXT    NOT NULL DEFAULT '',
    charged_nano_usd         INTEGER NOT NULL DEFAULT 0,
    estimated_nano_usd       INTEGER NOT NULL DEFAULT 0,
    reserved_nano_usd        INTEGER NOT NULL DEFAULT 0,
    price_snapshot           TEXT    NOT NULL DEFAULT '',
    latency_ms               INTEGER NOT NULL DEFAULT 0,
    attempts                 INTEGER NOT NULL DEFAULT 0,
    generation_id            TEXT    NOT NULL DEFAULT '',
    evidence_version         TEXT    NOT NULL,
    extractor_version        TEXT    NOT NULL,
    prompt_version           TEXT    NOT NULL,
    schema_version           TEXT    NOT NULL,
    taxonomy_version         TEXT    NOT NULL,
    admission_policy_version TEXT    NOT NULL,
    role_policy_version      TEXT    NOT NULL,
    capability_snapshot      TEXT    NOT NULL,
    outcome                  TEXT    NOT NULL DEFAULT '' CHECK (outcome IN ('', 'window_complete')),
    failure_reason           TEXT    NOT NULL DEFAULT '',
    created_at               INTEGER NOT NULL,
    updated_at               INTEGER NOT NULL
);
CREATE TABLE filler_structure_window_call_ledger (
    request_sha256   TEXT    NOT NULL,
    attempt          INTEGER NOT NULL DEFAULT 1,
    evaluation_id    TEXT    NOT NULL UNIQUE REFERENCES filler_inference_evaluations(id),
    source_sha256    TEXT    NOT NULL,
    media_set_sha256 TEXT    NOT NULL,
    window_ordinal   INTEGER NOT NULL,
    assessor_id      TEXT    NOT NULL,
    state            TEXT    NOT NULL,
    reservation_json TEXT    NOT NULL CHECK (json_valid(reservation_json)),
    record_sha256    TEXT    NOT NULL DEFAULT '',
    record_json      TEXT    CHECK (json_valid(record_json)),
    requested_at     INTEGER NOT NULL,
    assessed_at      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (request_sha256, attempt),
    CHECK (attempt >= 1),
    CHECK (window_ordinal >= 0),
    CHECK (state IN ('open', 'held_budget', 'settled')),
    CHECK (
        (state = 'settled' AND record_sha256 <> '' AND record_json IS NOT NULL AND assessed_at > 0) OR
        (state <> 'settled' AND record_sha256 = '' AND record_json IS NULL AND assessed_at = 0)
    )
);
INSERT INTO filler_inference_evaluations SELECT * FROM filler_inference_evaluations_old;
INSERT INTO filler_structure_window_call_ledger (request_sha256, attempt, evaluation_id, source_sha256,
    media_set_sha256, window_ordinal, assessor_id, state, reservation_json, record_sha256, record_json,
    requested_at, assessed_at)
SELECT request_sha256, attempt, evaluation_id, source_sha256,
    media_set_sha256, window_ordinal, assessor_id, state, reservation_json, record_sha256, NULLIF(record_json, ''),
    requested_at, assessed_at
  FROM filler_structure_window_call_ledger_old;
DROP TABLE filler_structure_window_call_ledger_old;
DROP TABLE filler_inference_evaluations_old;
CREATE INDEX idx_filler_inference_evaluations_clip_created
    ON filler_inference_evaluations (clip_hash, created_at DESC, id DESC);
CREATE INDEX idx_filler_inference_evaluations_cache_key
    ON filler_inference_evaluations (cache_key, created_at DESC, id DESC);
CREATE INDEX idx_filler_inference_evaluations_run_created
    ON filler_inference_evaluations (run_id, created_at DESC, id DESC);
CREATE INDEX idx_filler_inference_evaluations_day_budget
    ON filler_inference_evaluations (created_at, reserved_nano_usd);
CREATE INDEX idx_filler_structure_window_call_ledger_source
    ON filler_structure_window_call_ledger (source_sha256, media_set_sha256, requested_at DESC, request_sha256);
CREATE INDEX idx_filler_structure_window_call_ledger_open
    ON filler_structure_window_call_ledger (state, requested_at ASC, request_sha256);

-- filler_pulls and its commit ------------------------------------------------------------------
ALTER TABLE filler_pull_commits RENAME TO filler_pull_commits_old;
ALTER TABLE filler_pulls RENAME TO filler_pulls_old;
CREATE TABLE filler_pulls (
    id          TEXT    PRIMARY KEY,
    title       TEXT    NOT NULL,
    reason      TEXT    NOT NULL DEFAULT '',
    proposed_by TEXT    NOT NULL DEFAULT '',
    -- A decided pull is kept, not deleted: the queue's History answers what was agreed and when.
    status      TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'dismissed')),
    note        TEXT    NOT NULL DEFAULT '',
    -- ⚠ Rows the operator dropped stay in the plan, flagged: the record shows what was proposed
    -- and what was agreed to.
    plan_json   TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(plan_json)),
    created_at  INTEGER NOT NULL,
    decided_at  INTEGER NOT NULL DEFAULT 0,
    decided_by  TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE filler_pull_commits (
    pull_id        TEXT PRIMARY KEY REFERENCES filler_pulls(id),
    acquisition_id TEXT NOT NULL UNIQUE REFERENCES filler_acquisition_runs(id)
);
INSERT INTO filler_pulls SELECT * FROM filler_pulls_old;
INSERT INTO filler_pull_commits SELECT * FROM filler_pull_commits_old;
DROP TABLE filler_pull_commits_old;
DROP TABLE filler_pulls_old;
CREATE INDEX idx_filler_pulls_status ON filler_pulls (status, created_at);

-- inventory origins and analysis ---------------------------------------------------------------
ALTER TABLE inventory_external_ids RENAME TO inventory_external_ids_old;
ALTER TABLE inventory_source_origins RENAME TO inventory_source_origins_old;
ALTER TABLE inventory_item_origins RENAME TO inventory_item_origins_old;
ALTER TABLE inventory_source_analysis RENAME TO inventory_source_analysis_old;
ALTER TABLE inventory_source_measurements RENAME TO inventory_source_measurements_old;
CREATE TABLE inventory_item_origins (
    authority_id     TEXT   NOT NULL,
    external_item_id TEXT   NOT NULL,
    item_id          TEXT   NOT NULL REFERENCES inventory_items(id) ON DELETE CASCADE,
    observation_json TEXT   NOT NULL CHECK (json_valid(observation_json)),
    observed_at      BIGINT NOT NULL,
    last_seen_at     BIGINT NOT NULL,
    missing_at       BIGINT,
    PRIMARY KEY (authority_id, external_item_id)
);
CREATE TABLE inventory_external_ids (
    item_id          TEXT NOT NULL REFERENCES inventory_items(id) ON DELETE CASCADE,
    authority_id     TEXT NOT NULL,
    external_item_id TEXT NOT NULL,
    namespace        TEXT NOT NULL,
    external_id      TEXT NOT NULL,
    PRIMARY KEY (authority_id, external_item_id, namespace, external_id),
    FOREIGN KEY (authority_id, external_item_id)
        REFERENCES inventory_item_origins(authority_id, external_item_id) ON DELETE CASCADE
);
CREATE TABLE inventory_source_origins (
    authority_id       TEXT   NOT NULL,
    external_item_id   TEXT   NOT NULL,
    external_source_id TEXT   NOT NULL,
    source_id          TEXT   NOT NULL REFERENCES inventory_sources(id) ON DELETE CASCADE,
    locator_json       TEXT   NOT NULL CHECK (json_valid(locator_json)),
    observation_json   TEXT   NOT NULL CHECK (json_valid(observation_json)),
    observed_at        BIGINT NOT NULL,
    last_seen_at       BIGINT NOT NULL,
    missing_at         BIGINT,
    PRIMARY KEY (authority_id, external_item_id, external_source_id),
    FOREIGN KEY (authority_id, external_item_id)
        REFERENCES inventory_item_origins(authority_id, external_item_id) ON DELETE CASCADE
);
CREATE TABLE inventory_source_analysis (
    source_id           TEXT    PRIMARY KEY REFERENCES inventory_sources(id) ON DELETE CASCADE,
    source_revision     TEXT    NOT NULL,
    schema_version      INTEGER NOT NULL,
    keyframes           BLOB    NOT NULL,
    keyframe_count      INTEGER NOT NULL,
    integrated_lufs     DOUBLE PRECISION,
    true_peak_dbtp      DOUBLE PRECISION,
    breaks_json         TEXT    NOT NULL CHECK (json_valid(breaks_json)),
    analyzed_at         BIGINT  NOT NULL,
    active_picture_json TEXT    CHECK (json_valid(active_picture_json))
);
CREATE TABLE inventory_source_measurements (
    source_id        TEXT   PRIMARY KEY REFERENCES inventory_sources(id) ON DELETE CASCADE,
    source_revision  TEXT   NOT NULL,
    observation_json TEXT   NOT NULL CHECK (json_valid(observation_json)),
    observed_at      BIGINT NOT NULL
);
INSERT INTO inventory_item_origins SELECT * FROM inventory_item_origins_old;
INSERT INTO inventory_external_ids SELECT * FROM inventory_external_ids_old;
INSERT INTO inventory_source_origins SELECT * FROM inventory_source_origins_old;
INSERT INTO inventory_source_analysis SELECT * FROM inventory_source_analysis_old;
INSERT INTO inventory_source_measurements SELECT * FROM inventory_source_measurements_old;
DROP TABLE inventory_external_ids_old;
DROP TABLE inventory_source_origins_old;
DROP TABLE inventory_item_origins_old;
DROP TABLE inventory_source_analysis_old;
DROP TABLE inventory_source_measurements_old;
CREATE INDEX idx_inventory_item_origins_item ON inventory_item_origins (item_id);
CREATE INDEX idx_inventory_external_ids_grounded
    ON inventory_external_ids (namespace, external_id, item_id);
CREATE INDEX idx_inventory_external_ids_item ON inventory_external_ids (item_id);
CREATE INDEX idx_inventory_source_origins_source ON inventory_source_origins (source_id);

-- notifications --------------------------------------------------------------------------------
ALTER TABLE notification_delivery_attempts RENAME TO notification_delivery_attempts_old;
ALTER TABLE notification_intents RENAME TO notification_intents_old;
ALTER TABLE notification_destinations RENAME TO notification_destinations_old;
CREATE TABLE notification_destinations (
    id                       TEXT    PRIMARY KEY,
    means                    TEXT    NOT NULL CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    )),
    label                    TEXT    NOT NULL,
    scope                    TEXT    NOT NULL CHECK (scope IN ('installation', 'person')),
    owner_id                 TEXT    NOT NULL DEFAULT '',
    audience                 TEXT    NOT NULL CHECK (audience IN ('person', 'approvers', 'operators')),
    topics_json              TEXT    NOT NULL CHECK (json_valid(topics_json)),
    enabled                  BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
    configuration_json       TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(configuration_json)),
    credentials_encrypted    TEXT    NOT NULL,
    created_at               BIGINT  NOT NULL,
    updated_at               BIGINT  NOT NULL,
    credential_keys_json     TEXT    CHECK (json_valid(credential_keys_json)),
    subscription_fingerprint TEXT    NOT NULL DEFAULT '',
    CHECK (
        (scope = 'installation' AND owner_id = '' AND audience IN ('approvers', 'operators')) OR
        (scope = 'person' AND owner_id <> '' AND audience IN ('person', 'approvers', 'operators'))
    )
);
CREATE TABLE notification_intents (
    id                TEXT PRIMARY KEY,
    topic             TEXT NOT NULL CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_denied',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded',
        'delivery_test',
        'library_folder_offline', 'library_folder_back', 'library_changes_degraded',
        'library_needs_you'
    )),
    recipient_kind    TEXT NOT NULL CHECK (recipient_kind IN ('invitation', 'person', 'approvers', 'operators')),
    recipient_id      TEXT NOT NULL,
    reference_kind    TEXT NOT NULL CHECK (reference_kind IN ('invitation', 'recovery', 'proposal', 'title', 'channel', 'destination', 'library')),
    reference_id      TEXT NOT NULL,
    recipient_policy  TEXT NOT NULL CHECK (recipient_policy IN ('mandatory_account', 'configurable')),
    template_json     TEXT NOT NULL CHECK (json_valid(template_json)),
    idempotency_key   TEXT NOT NULL UNIQUE,
    created_at        BIGINT NOT NULL,
    terminal_at       BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE notification_delivery_attempts (
    id                    TEXT PRIMARY KEY,
    intent_id             TEXT NOT NULL REFERENCES notification_intents(id) ON DELETE CASCADE,
    means                 TEXT NOT NULL CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    )),
    destination_ref       TEXT NOT NULL,
    destination_redacted  TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (status IN ('queued', 'sending', 'delivered', 'failed', 'suppressed')),
    attempt_number        INTEGER NOT NULL CHECK (attempt_number BETWEEN 1 AND 5),
    available_at          BIGINT NOT NULL,
    lease_owner           TEXT NOT NULL DEFAULT '',
    lease_expires_at      BIGINT NOT NULL DEFAULT 0,
    started_at            BIGINT NOT NULL DEFAULT 0,
    finished_at           BIGINT NOT NULL DEFAULT 0,
    provider_message_id   TEXT NOT NULL DEFAULT '',
    failure_class         TEXT NOT NULL DEFAULT '' CHECK (failure_class IN ('', 'transient_pre_acceptance', 'permanent', 'ambiguous_acceptance', 'cancelled')),
    outcome_code          TEXT NOT NULL DEFAULT '' CHECK (outcome_code IN ('', 'delivery_disabled', 'destination_unavailable', 'preference_disabled', 'means_unavailable', 'recipient_rejected', 'configuration_invalid', 'transport_unavailable', 'acceptance_ambiguous', 'cancelled', 'worker_interrupted')),
    created_at            BIGINT NOT NULL,
    UNIQUE (intent_id, means, destination_ref, attempt_number),
    CHECK (
        (status = 'queued' AND lease_owner = '' AND lease_expires_at = 0 AND started_at = 0 AND finished_at = 0 AND provider_message_id = '' AND failure_class = '' AND outcome_code = '') OR
        (status = 'sending' AND lease_owner <> '' AND lease_expires_at > 0 AND started_at > 0 AND finished_at = 0 AND provider_message_id = '' AND failure_class = '' AND outcome_code = '') OR
        (status = 'delivered' AND lease_owner = '' AND lease_expires_at = 0 AND finished_at > 0 AND failure_class = '' AND outcome_code = '') OR
        (status = 'failed' AND lease_owner = '' AND lease_expires_at = 0 AND finished_at > 0 AND provider_message_id = '' AND failure_class <> '' AND outcome_code <> '') OR
        (status = 'suppressed' AND lease_owner = '' AND lease_expires_at = 0 AND finished_at > 0 AND provider_message_id = '' AND failure_class = '' AND outcome_code IN ('delivery_disabled', 'destination_unavailable', 'preference_disabled'))
    )
);
INSERT INTO notification_destinations SELECT * FROM notification_destinations_old;
INSERT INTO notification_intents SELECT * FROM notification_intents_old;
INSERT INTO notification_delivery_attempts SELECT * FROM notification_delivery_attempts_old;
DROP TABLE notification_delivery_attempts_old;
DROP TABLE notification_intents_old;
DROP TABLE notification_destinations_old;
CREATE INDEX idx_notification_destinations_routing
    ON notification_destinations (enabled, scope, audience, means, id);
CREATE UNIQUE INDEX idx_notification_destinations_web_push_subscription
    ON notification_destinations (owner_id, subscription_fingerprint)
    WHERE means = 'web_push' AND subscription_fingerprint <> '';
CREATE INDEX idx_notification_intents_terminal
    ON notification_intents (terminal_at) WHERE terminal_at > 0;
CREATE INDEX idx_notification_intents_reference
    ON notification_intents (reference_kind, reference_id, created_at DESC, id DESC);
CREATE INDEX idx_notification_delivery_attempts_due
    ON notification_delivery_attempts (available_at, id) WHERE status = 'queued';

-- proposals, their jobs and attempts; titles; series_episodes ----------------------------------
ALTER TABLE proposal_job_attempts RENAME TO proposal_job_attempts_old;
ALTER TABLE proposal_jobs RENAME TO proposal_jobs_old;
ALTER TABLE proposals RENAME TO proposals_old;
ALTER TABLE titles RENAME TO titles_old;
ALTER TABLE series_episodes RENAME TO series_episodes_old;
CREATE TABLE proposal_jobs (
    id                 TEXT    PRIMARY KEY,
    kind               TEXT    NOT NULL DEFAULT 'suggest' CHECK (kind IN ('suggest', 'recurate')),
    status             TEXT    NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    intent_json        TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(intent_json)),
    intent_hash        TEXT    NOT NULL DEFAULT '',
    created_by         TEXT    NOT NULL DEFAULT '',
    last_error         TEXT    NOT NULL DEFAULT '',
    -- deadline drives ClaimDueJobs: a queued job is due at or before now; claiming leases it so
    -- two workers or replicas don't run it twice.
    deadline           BIGINT  NOT NULL DEFAULT 0,
    attempts           INTEGER NOT NULL DEFAULT 0,
    created_at         BIGINT  NOT NULL DEFAULT 0,
    updated_at         BIGINT  NOT NULL DEFAULT 0,
    failure_code       TEXT    NOT NULL DEFAULT '',
    workflow_version   INTEGER NOT NULL DEFAULT 0,
    reached_live       BOOLEAN NOT NULL DEFAULT FALSE CHECK (reached_live IN (0, 1)),
    failure_trace_json TEXT    CHECK (json_valid(failure_trace_json)),
    channel_id         TEXT    NOT NULL DEFAULT ''
);
CREATE TABLE proposal_job_attempts (
    job_id           TEXT    NOT NULL REFERENCES proposal_jobs(id) ON DELETE CASCADE,
    attempt          INTEGER NOT NULL,
    workflow_version INTEGER NOT NULL,
    status           TEXT    NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'interrupted')),
    started_at       BIGINT  NOT NULL,
    completed_at     BIGINT  NOT NULL DEFAULT 0,
    failure_code     TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (job_id, attempt)
);
CREATE TABLE proposals (
    id            TEXT   PRIMARY KEY,
    job_id        TEXT   NOT NULL DEFAULT '',
    status        TEXT   NOT NULL CHECK (status IN ('submitted', 'approved', 'denied', 'superseded')),
    created_by    TEXT   NOT NULL DEFAULT '',
    decided_by    TEXT   NOT NULL DEFAULT '',
    deny_reason   TEXT   NOT NULL DEFAULT '',
    proposal_json TEXT   NOT NULL DEFAULT '{}' CHECK (json_valid(proposal_json)),
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0,
    mod_summary   TEXT   NOT NULL DEFAULT '',
    note          TEXT   NOT NULL DEFAULT '',
    approved_at   BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE titles (
    key             TEXT    PRIMARY KEY,
    title_json      TEXT    NOT NULL CHECK (json_valid(title_json)),
    state           TEXT    NOT NULL CHECK (state IN ('wanted', 'requested', 'downloading', 'available', 'unavailable')),
    library_id      TEXT    NOT NULL DEFAULT '',
    requested_at    BIGINT  NOT NULL DEFAULT 0,
    deadline        BIGINT  NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT    NOT NULL DEFAULT '',
    updated_at      BIGINT  NOT NULL DEFAULT 0,
    progress        REAL    NOT NULL DEFAULT 0,
    eta_text        TEXT    NOT NULL DEFAULT '',
    download_status TEXT    NOT NULL DEFAULT '',
    arrived_at      BIGINT  NOT NULL DEFAULT 0,
    episodes_have   INTEGER NOT NULL DEFAULT 0,
    episodes_wanted INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE series_episodes (
    library_id    TEXT    PRIMARY KEY,          -- media-server SHOW item id
    episodes_json TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(episodes_json)),
    episode_count INTEGER NOT NULL DEFAULT 0,   -- denormalized for cheap diagnostics/logging
    fetched_at    BIGINT  NOT NULL DEFAULT 0    -- unix seconds; 0 = never (treated as stale)
);
INSERT INTO proposal_jobs (id, kind, status, intent_json, intent_hash, created_by, last_error, deadline,
    attempts, created_at, updated_at, failure_code, workflow_version, reached_live, failure_trace_json,
    channel_id)
SELECT id, kind, status, intent_json, intent_hash, created_by, last_error, deadline,
    attempts, created_at, updated_at, failure_code, workflow_version, reached_live,
    NULLIF(failure_trace_json, ''), channel_id
  FROM proposal_jobs_old;
INSERT INTO proposal_job_attempts SELECT * FROM proposal_job_attempts_old;
INSERT INTO proposals SELECT * FROM proposals_old;
INSERT INTO titles SELECT * FROM titles_old;
INSERT INTO series_episodes SELECT * FROM series_episodes_old;
DROP TABLE proposal_job_attempts_old;
DROP TABLE proposal_jobs_old;
DROP TABLE proposals_old;
DROP TABLE titles_old;
DROP TABLE series_episodes_old;
CREATE INDEX idx_proposal_jobs_status_deadline ON proposal_jobs (status, deadline);
CREATE INDEX idx_proposal_jobs_intent_hash ON proposal_jobs (intent_hash);
CREATE INDEX idx_proposal_job_attempts_job_started ON proposal_job_attempts (job_id, started_at DESC);
CREATE INDEX idx_proposals_status ON proposals (status);
CREATE INDEX idx_proposals_created_by ON proposals (created_by);
CREATE INDEX idx_proposals_job_id ON proposals (job_id);
CREATE INDEX idx_titles_state_deadline ON titles (state, deadline);
CREATE INDEX idx_titles_arrived ON titles (arrived_at);
CREATE INDEX idx_series_episodes_fetched_at ON series_episodes (fetched_at);

-- taxa and its holidays ------------------------------------------------------------------------
ALTER TABLE holidays RENAME TO holidays_old;
ALTER TABLE taxa RENAME TO taxa_old;
CREATE TABLE taxa (
    slug       TEXT    PRIMARY KEY,
    label      TEXT    NOT NULL,
    parent     TEXT    NOT NULL DEFAULT '',
    axis       TEXT    NOT NULL,
    synonyms   TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(synonyms)),
    aliases    TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(aliases)),  -- retired slugs
    updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE holidays (
    id                       TEXT    PRIMARY KEY REFERENCES taxa(slug),
    label                    TEXT    NOT NULL,
    position                 INTEGER NOT NULL,
    words                    TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(words)),
    kept                     TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(kept)),
    air_window               TEXT    CHECK (json_valid(air_window)),
    members                  TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(members)),
    name_needs_corroboration BOOLEAN NOT NULL DEFAULT FALSE CHECK (name_needs_corroboration IN (0, 1)),
    updated_at               INTEGER NOT NULL DEFAULT 0
);
INSERT INTO taxa SELECT * FROM taxa_old;
INSERT INTO holidays (id, label, position, words, kept, air_window, members, name_needs_corroboration, updated_at)
SELECT id, label, position, words, kept, NULLIF(air_window, ''), members, name_needs_corroboration, updated_at
  FROM holidays_old;
DROP TABLE holidays_old;
DROP TABLE taxa_old;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
