-- +goose Up
-- Schema naming (#2323): the core state columns accept only the states the code knows, so a
-- misspelt state fails at the write instead of parking a row where no query looks.
-- SQLite cannot add a CHECK to an existing column, so each table is rebuilt transactionally.
-- proposal_job_attempts references proposal_jobs, so it is rebuilt with it (the 00152 pattern).

-- titles
DROP INDEX idx_titles_state_deadline;
DROP INDEX idx_titles_arrived;
ALTER TABLE titles RENAME TO titles_old;
CREATE TABLE titles (
    key             TEXT PRIMARY KEY,
    title_json      TEXT NOT NULL,
    state           TEXT NOT NULL CHECK (state IN ('wanted', 'requested', 'downloading', 'available', 'unavailable')),
    library_id      TEXT NOT NULL DEFAULT '',
    requested_at    BIGINT NOT NULL DEFAULT 0,
    deadline        BIGINT NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    updated_at      BIGINT NOT NULL DEFAULT 0,
    progress        REAL NOT NULL DEFAULT 0,
    eta_text        TEXT NOT NULL DEFAULT '',
    download_status TEXT NOT NULL DEFAULT '',
    arrived_at      BIGINT NOT NULL DEFAULT 0,
    episodes_have   INTEGER NOT NULL DEFAULT 0,
    episodes_wanted INTEGER NOT NULL DEFAULT 0
);
INSERT INTO titles (key, title_json, state, library_id, requested_at, deadline, attempts, last_error,
    updated_at, progress, eta_text, download_status, arrived_at, episodes_have, episodes_wanted)
SELECT key, title_json, state, library_id, requested_at, deadline, attempts, last_error,
    updated_at, progress, eta_text, download_status, arrived_at, episodes_have, episodes_wanted
  FROM titles_old;
DROP TABLE titles_old;
CREATE INDEX idx_titles_state_deadline ON titles (state, deadline);
CREATE INDEX idx_titles_arrived ON titles (arrived_at);

-- proposals
DROP INDEX idx_proposals_status;
DROP INDEX idx_proposals_created_by;
DROP INDEX idx_proposals_job_id;
ALTER TABLE proposals RENAME TO proposals_old;
CREATE TABLE proposals (
    id            TEXT PRIMARY KEY,
    job_id        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL CHECK (status IN ('submitted', 'approved', 'denied', 'superseded')),
    created_by    TEXT NOT NULL DEFAULT '',
    decided_by    TEXT NOT NULL DEFAULT '',
    deny_reason   TEXT NOT NULL DEFAULT '',
    proposal_json TEXT NOT NULL DEFAULT '{}',
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0,
    mod_summary   TEXT NOT NULL DEFAULT '',
    note          TEXT NOT NULL DEFAULT '',
    approved_at   BIGINT NOT NULL DEFAULT 0
);
INSERT INTO proposals (id, job_id, status, created_by, decided_by, deny_reason, proposal_json,
    created_at, updated_at, mod_summary, note, approved_at)
SELECT id, job_id, status, created_by, decided_by, deny_reason, proposal_json,
    created_at, updated_at, mod_summary, note, approved_at
  FROM proposals_old;
DROP TABLE proposals_old;
CREATE INDEX idx_proposals_status ON proposals (status);
CREATE INDEX idx_proposals_created_by ON proposals (created_by);
CREATE INDEX idx_proposals_job_id ON proposals (job_id);

-- proposal_jobs and its attempts
DROP INDEX idx_proposal_jobs_status_deadline;
DROP INDEX idx_proposal_jobs_intent_hash;
DROP INDEX idx_proposal_job_attempts_job_started;
ALTER TABLE proposal_job_attempts RENAME TO proposal_job_attempts_old;
ALTER TABLE proposal_jobs RENAME TO proposal_jobs_old;
CREATE TABLE proposal_jobs (
    id                 TEXT PRIMARY KEY,
    kind               TEXT NOT NULL DEFAULT 'suggest',
    status             TEXT NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    intent_json        TEXT NOT NULL DEFAULT '{}',
    intent_hash        TEXT NOT NULL DEFAULT '',
    created_by         TEXT NOT NULL DEFAULT '',
    last_error         TEXT NOT NULL DEFAULT '',
    -- deadline drives ClaimDueJobs: a queued job is due at/before now; claiming
    -- leases it (deadline := now+lease) so two workers/replicas don't run it twice.
    deadline           BIGINT NOT NULL DEFAULT 0,
    attempts           INTEGER NOT NULL DEFAULT 0,
    created_at         BIGINT NOT NULL DEFAULT 0,
    updated_at         BIGINT NOT NULL DEFAULT 0,
    failure_code       TEXT NOT NULL DEFAULT '',
    workflow_version   INTEGER NOT NULL DEFAULT 0,
    reached_live       BOOLEAN NOT NULL DEFAULT 0,
    failure_trace_json TEXT NOT NULL DEFAULT '',
    channel_id         TEXT NOT NULL DEFAULT ''
);
CREATE TABLE proposal_job_attempts (
    job_id            TEXT NOT NULL REFERENCES proposal_jobs(id) ON DELETE CASCADE,
    attempt           INTEGER NOT NULL,
    workflow_version  INTEGER NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'interrupted')),
    started_at        BIGINT NOT NULL,
    completed_at      BIGINT NOT NULL DEFAULT 0,
    failure_code      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (job_id, attempt)
);
INSERT INTO proposal_jobs (id, kind, status, intent_json, intent_hash, created_by, last_error, deadline,
    attempts, created_at, updated_at, failure_code, workflow_version, reached_live, failure_trace_json,
    channel_id)
SELECT id, kind, status, intent_json, intent_hash, created_by, last_error, deadline,
    attempts, created_at, updated_at, failure_code, workflow_version, reached_live, failure_trace_json,
    channel_id
  FROM proposal_jobs_old;
INSERT INTO proposal_job_attempts (job_id, attempt, workflow_version, status, started_at, completed_at,
    failure_code)
SELECT job_id, attempt, workflow_version, status, started_at, completed_at, failure_code
  FROM proposal_job_attempts_old;
DROP TABLE proposal_job_attempts_old;
DROP TABLE proposal_jobs_old;
CREATE INDEX idx_proposal_jobs_status_deadline ON proposal_jobs (status, deadline);
CREATE INDEX idx_proposal_jobs_intent_hash ON proposal_jobs (intent_hash);
CREATE INDEX idx_proposal_job_attempts_job_started
    ON proposal_job_attempts (job_id, started_at DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
