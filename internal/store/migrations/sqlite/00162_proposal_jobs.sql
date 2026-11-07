-- +goose Up
-- Schema naming (#2323, #2321): `jobs` holds only Proposal Jobs (a suggestion or a scheduled
-- re-curation), while background work is River's. The table says so, like its
-- `proposal_job_attempts` child, whose foreign key follows the rename. Indexes take the
-- `idx_<table>_<purpose>` form. SQLite has no ALTER INDEX, so they are recreated.
ALTER TABLE jobs RENAME TO proposal_jobs;
DROP INDEX idx_jobs_status_deadline;
DROP INDEX idx_jobs_intent_hash;
CREATE INDEX idx_proposal_jobs_status_deadline ON proposal_jobs (status, deadline);
CREATE INDEX idx_proposal_jobs_intent_hash ON proposal_jobs (intent_hash);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
