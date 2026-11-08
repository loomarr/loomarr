-- +goose Up
-- Existing jobs predate exact attempt history. Version 0 remains readable; the
-- first post-upgrade claim promotes a job to version 1 and starts truthful,
-- per-attempt history instead of inventing details for past executions.
ALTER TABLE jobs ADD COLUMN workflow_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN reached_live BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE proposal_job_attempts (
    job_id            TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt           INTEGER NOT NULL,
    workflow_version  INTEGER NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'interrupted')),
    started_at        BIGINT NOT NULL,
    completed_at      BIGINT NOT NULL DEFAULT 0,
    failure_code      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (job_id, attempt)
);

CREATE INDEX idx_proposal_job_attempts_job_started
    ON proposal_job_attempts (job_id, started_at DESC);

-- +goose Down
DROP TABLE proposal_job_attempts;
ALTER TABLE jobs DROP COLUMN reached_live;
ALTER TABLE jobs DROP COLUMN workflow_version;
