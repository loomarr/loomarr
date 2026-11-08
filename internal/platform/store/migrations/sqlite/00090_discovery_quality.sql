-- +goose Up
CREATE TABLE quality_run_snapshots (
    id TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL,
    corpus_version TEXT NOT NULL,
    requested_model TEXT NOT NULL,
    resolved_model TEXT NOT NULL,
    provider TEXT NOT NULL,
    budget_profile TEXT NOT NULL,
    application_version TEXT NOT NULL,
    accounting_available BOOLEAN NOT NULL,
    created_at BIGINT NOT NULL
);

CREATE TABLE quality_receipts (
    idempotency_key TEXT PRIMARY KEY,
    occurred_at BIGINT NOT NULL,
    stage TEXT NOT NULL,
    outcome TEXT NOT NULL,
    duration_millis BIGINT NOT NULL,
    tool_calls INTEGER NOT NULL,
    candidate_count INTEGER NOT NULL,
    cost_nanos BIGINT NOT NULL,
    run_snapshot_id TEXT REFERENCES quality_run_snapshots(id)
);
CREATE INDEX idx_quality_receipts_rollup ON quality_receipts (occurred_at, stage, outcome);

CREATE TABLE quality_daily_aggregates (
    day TEXT NOT NULL,
    stage TEXT NOT NULL,
    outcome TEXT NOT NULL,
    run_snapshot_id TEXT NOT NULL DEFAULT '',
    observation_count BIGINT NOT NULL,
    duration_millis BIGINT NOT NULL,
    tool_calls BIGINT NOT NULL,
    candidate_count BIGINT NOT NULL,
    cost_nanos BIGINT NOT NULL,
    PRIMARY KEY (day, stage, outcome, run_snapshot_id)
);

-- +goose Down
DROP TABLE IF EXISTS quality_daily_aggregates;
DROP INDEX IF EXISTS idx_quality_receipts_rollup;
DROP TABLE IF EXISTS quality_receipts;
DROP TABLE IF EXISTS quality_run_snapshots;
