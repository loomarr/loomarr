-- +goose Up
-- Schema naming (#2323, #2321): `jobs` holds only Proposal Jobs (a suggestion or a scheduled
-- re-curation), while background work is River's. The table says so, like its
-- `proposal_job_attempts` child, whose foreign key follows the rename. Indexes take the
-- `idx_<table>_<purpose>` form.
ALTER TABLE jobs RENAME TO proposal_jobs;
ALTER INDEX jobs_pkey RENAME TO proposal_jobs_pkey;
ALTER INDEX idx_jobs_status_deadline RENAME TO idx_proposal_jobs_status_deadline;
ALTER INDEX idx_jobs_intent_hash RENAME TO idx_proposal_jobs_intent_hash;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
