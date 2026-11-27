-- +goose Up
-- Schema to a 10 (#2793, item 1): the people on a suggestion are foreign keys to users.
--
--   - proposals.created_by and proposal_jobs.created_by, the member who asked;
--   - proposals.decided_by, the member who approved or denied;
--   - discovery_feedback.actor_id, the member who gave the feedback.
--
-- Each references users and is cleared when that person is deleted (SET NULL): a request, a
-- decision and feedback outlive the person, as the audit trail needs. The API token is no
-- person, so what it writes ('' before, 'api-token' for feedback) is NULL now, and so is an id
-- naming no user.
--
-- decided_by also held 'auto' and 'auto-curate' for a decision no person made. Those move to the
-- new proposals.decided_via ('person' | 'auto' | 'auto-curate'), which says how every approved or
-- denied suggestion was decided; decided_by then names only a member.
--
-- proposals.job_id, the request a suggestion answered, is a foreign key to proposal_jobs too, and
-- is cleared when the nightly purge removes that finished job (SET NULL). The purge already keeps a
-- job while a suggestion from it is still open, so only a decided suggestion forgets its job. ''
-- (no job) and the id of a job already purged become NULL.
--
-- SQLite adds a foreign key to an existing column by adding a keyed column, copying the values
-- across, and dropping the old one under its name (the 00211 pattern).

-- proposals ---------------------------------------------------------------------------------------
ALTER TABLE proposals ADD COLUMN decided_via TEXT CHECK (decided_via IN ('person', 'auto', 'auto-curate'));
UPDATE proposals SET decided_via = CASE
    WHEN decided_by IN ('auto', 'auto-curate') THEN decided_by
    WHEN status IN ('approved', 'denied') THEN 'person'
END;

ALTER TABLE proposals ADD COLUMN creator TEXT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE proposals ADD COLUMN decider TEXT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE proposals ADD COLUMN job TEXT REFERENCES proposal_jobs(id) ON DELETE SET NULL;
UPDATE proposals SET
    creator = (SELECT u.id FROM users u WHERE u.id = proposals.created_by),
    decider = (SELECT u.id FROM users u WHERE u.id = proposals.decided_by),
    job = (SELECT j.id FROM proposal_jobs j WHERE j.id = proposals.job_id);
DROP INDEX idx_proposals_created_by;
DROP INDEX idx_proposals_job_id;
ALTER TABLE proposals DROP COLUMN created_by;
ALTER TABLE proposals DROP COLUMN decided_by;
ALTER TABLE proposals DROP COLUMN job_id;
ALTER TABLE proposals RENAME COLUMN creator TO created_by;
ALTER TABLE proposals RENAME COLUMN decider TO decided_by;
ALTER TABLE proposals RENAME COLUMN job TO job_id;
CREATE INDEX idx_proposals_created_by ON proposals (created_by);
CREATE INDEX idx_proposals_job_id ON proposals (job_id);
CREATE INDEX idx_proposals_decided_by ON proposals (decided_by);

-- proposal_jobs -----------------------------------------------------------------------------------
ALTER TABLE proposal_jobs ADD COLUMN creator TEXT REFERENCES users(id) ON DELETE SET NULL;
UPDATE proposal_jobs SET creator = (SELECT u.id FROM users u WHERE u.id = proposal_jobs.created_by);
ALTER TABLE proposal_jobs DROP COLUMN created_by;
ALTER TABLE proposal_jobs RENAME COLUMN creator TO created_by;
CREATE INDEX idx_proposal_jobs_created_by ON proposal_jobs (created_by);

-- discovery_feedback ------------------------------------------------------------------------------
ALTER TABLE discovery_feedback ADD COLUMN actor TEXT REFERENCES users(id) ON DELETE SET NULL;
UPDATE discovery_feedback SET actor = (SELECT u.id FROM users u WHERE u.id = discovery_feedback.actor_id);
ALTER TABLE discovery_feedback DROP COLUMN actor_id;
ALTER TABLE discovery_feedback RENAME COLUMN actor TO actor_id;
CREATE INDEX idx_discovery_feedback_actor ON discovery_feedback (actor_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
