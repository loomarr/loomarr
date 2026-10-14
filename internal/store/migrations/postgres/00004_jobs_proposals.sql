-- +goose Up
-- Postgres mirror of the jobs + proposals schema (§8). See the sqlite variant for
-- commentary; kept ANSI-compatible. Epoch BIGINT timestamps (§3). ClaimDueJobs
-- uses FOR UPDATE SKIP LOCKED against the deadline for multi-replica single-run
-- job claiming (§8/§18).

CREATE TABLE jobs (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL DEFAULT 'suggest',
    status       TEXT NOT NULL,
    intent_json  TEXT NOT NULL DEFAULT '{}',
    intent_hash  TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    last_error   TEXT NOT NULL DEFAULT '',
    deadline     BIGINT NOT NULL DEFAULT 0,
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   BIGINT NOT NULL DEFAULT 0,
    updated_at   BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_jobs_status_deadline ON jobs (status, deadline);
CREATE INDEX idx_jobs_intent_hash ON jobs (intent_hash);

CREATE TABLE proposals (
    id            TEXT PRIMARY KEY,
    job_id        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    created_by    TEXT NOT NULL DEFAULT '',
    approved_by   TEXT NOT NULL DEFAULT '',
    deny_reason   TEXT NOT NULL DEFAULT '',
    proposal_json TEXT NOT NULL DEFAULT '{}',
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_proposals_status ON proposals (status);
CREATE INDEX idx_proposals_created_by ON proposals (created_by);

-- +goose Down
DROP TABLE proposals;
DROP TABLE jobs;
