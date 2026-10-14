-- +goose Up
-- Suggester jobs + proposals (§8 execution model). Forward-only (§16). Epoch
-- BIGINT timestamps (§3). A job is a persisted generation task claimed by the
-- worker pool via ClaimDueJobs (same SKIP-LOCKED lease as titles/channels). A
-- proposal is the job's output, carrying created_by (My proposals) and status
-- (the approval queue) — both must survive restarts (§8).

CREATE TABLE jobs (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL DEFAULT 'suggest', -- suggest | (tag, filler-sync later)
    status       TEXT NOT NULL,                   -- queued | running | done | failed
    intent_json  TEXT NOT NULL DEFAULT '{}',      -- the suggest.Intent
    intent_hash  TEXT NOT NULL DEFAULT '',        -- cache key: hash(normalized intent)
    created_by   TEXT NOT NULL DEFAULT '',        -- user id who submitted (§8)
    last_error   TEXT NOT NULL DEFAULT '',
    -- deadline drives ClaimDueJobs: a queued job is due at/before now; claiming
    -- leases it (deadline := now+lease) so two workers/replicas don't run it twice.
    deadline     BIGINT NOT NULL DEFAULT 0,
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   BIGINT NOT NULL DEFAULT 0,
    updated_at   BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_jobs_status_deadline ON jobs (status, deadline);
CREATE INDEX idx_jobs_intent_hash ON jobs (intent_hash);

CREATE TABLE proposals (
    id            TEXT PRIMARY KEY,
    job_id        TEXT NOT NULL DEFAULT '',       -- the job that produced it
    status        TEXT NOT NULL,                  -- submitted | approved | denied
    created_by    TEXT NOT NULL DEFAULT '',       -- member who requested it (§12 My proposals)
    approved_by   TEXT NOT NULL DEFAULT '',       -- admin who approved/denied (§8 audit)
    deny_reason   TEXT NOT NULL DEFAULT '',
    proposal_json TEXT NOT NULL DEFAULT '{}',     -- the suggest.Proposal
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_proposals_status ON proposals (status);
CREATE INDEX idx_proposals_created_by ON proposals (created_by);

-- +goose Down
DROP TABLE proposals;
DROP TABLE jobs;
