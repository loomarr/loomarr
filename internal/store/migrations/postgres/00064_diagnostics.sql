-- +goose Up
-- Retained technical diagnostics (§5, §17). Activity remains the curated Dashboard feed;
-- these rows are the filterable evidence an operator or support agent correlates.
-- Forward-only (§16).
CREATE TABLE diagnostic_events (
    id                  TEXT   NOT NULL PRIMARY KEY,
    occurred_at         BIGINT NOT NULL,
    received_at         BIGINT NOT NULL,
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
    attributes_json     TEXT   NOT NULL DEFAULT '{}',
    size_bytes          BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_diagnostic_events_time ON diagnostic_events (occurred_at DESC, id DESC);
CREATE INDEX idx_diagnostic_events_level_time ON diagnostic_events (level, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_source_time ON diagnostic_events (source, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_subsystem_time ON diagnostic_events (subsystem, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_request ON diagnostic_events (request_id, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_playback ON diagnostic_events (playback_session_id, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_channel ON diagnostic_events (channel_id, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_schedule_block ON diagnostic_events (schedule_block_id, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_job ON diagnostic_events (job_id, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_process ON diagnostic_events (process_run_id, occurred_at DESC);
CREATE INDEX idx_diagnostic_events_instance ON diagnostic_events (instance_id, occurred_at DESC);

CREATE TABLE diagnostic_process_runs (
    id                   TEXT    NOT NULL PRIMARY KEY,
    purpose              TEXT    NOT NULL,
    parent_run_id        TEXT    NOT NULL DEFAULT '',
    instance_id          TEXT    NOT NULL DEFAULT '',
    channel_id           TEXT    NOT NULL DEFAULT '',
    target               TEXT    NOT NULL DEFAULT '',
    schedule_block_id    TEXT    NOT NULL DEFAULT '',
    job_id               TEXT    NOT NULL DEFAULT '',
    executable           TEXT    NOT NULL DEFAULT '',
    executable_version   TEXT    NOT NULL DEFAULT '',
    command_summary      TEXT    NOT NULL DEFAULT '',
    started_at           BIGINT  NOT NULL,
    ended_at             BIGINT  NOT NULL DEFAULT 0,
    status               TEXT    NOT NULL,
    exit_code            INTEGER,
    termination_reason   TEXT    NOT NULL DEFAULT '',
    first_error          TEXT    NOT NULL DEFAULT '',
    last_error           TEXT    NOT NULL DEFAULT '',
    output_ref           TEXT    NOT NULL DEFAULT '',
    output_bytes         BIGINT  NOT NULL DEFAULT 0,
    discarded_lines      BIGINT  NOT NULL DEFAULT 0,
    updated_at           BIGINT  NOT NULL,
    size_bytes           BIGINT  NOT NULL DEFAULT 0
);

CREATE INDEX idx_diagnostic_process_runs_time ON diagnostic_process_runs (started_at DESC, id DESC);
CREATE INDEX idx_diagnostic_process_runs_status_time ON diagnostic_process_runs (status, started_at DESC);
CREATE INDEX idx_diagnostic_process_runs_channel ON diagnostic_process_runs (channel_id, started_at DESC);
CREATE INDEX idx_diagnostic_process_runs_job ON diagnostic_process_runs (job_id, started_at DESC);
CREATE INDEX idx_diagnostic_process_runs_parent ON diagnostic_process_runs (parent_run_id, started_at DESC);
CREATE INDEX idx_diagnostic_process_runs_instance ON diagnostic_process_runs (instance_id, started_at DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
