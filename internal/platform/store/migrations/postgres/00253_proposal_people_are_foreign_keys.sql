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

-- proposals ---------------------------------------------------------------------------------------
ALTER TABLE proposals ADD COLUMN decided_via TEXT CHECK (decided_via IN ('person', 'auto', 'auto-curate'));
UPDATE proposals SET decided_via = CASE
    WHEN decided_by IN ('auto', 'auto-curate') THEN decided_by
    WHEN status IN ('approved', 'denied') THEN 'person'
END;

ALTER TABLE proposals
    ALTER COLUMN created_by DROP NOT NULL, ALTER COLUMN created_by DROP DEFAULT,
    ALTER COLUMN decided_by DROP NOT NULL, ALTER COLUMN decided_by DROP DEFAULT,
    ALTER COLUMN job_id DROP NOT NULL, ALTER COLUMN job_id DROP DEFAULT;
UPDATE proposals p SET
    created_by = (SELECT u.id FROM users u WHERE u.id = p.created_by),
    decided_by = (SELECT u.id FROM users u WHERE u.id = p.decided_by),
    job_id = (SELECT j.id FROM proposal_jobs j WHERE j.id = p.job_id);
ALTER TABLE proposals
    ADD CONSTRAINT proposals_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT proposals_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT proposals_job_id_fkey FOREIGN KEY (job_id) REFERENCES proposal_jobs(id) ON DELETE SET NULL;
CREATE INDEX idx_proposals_decided_by ON proposals (decided_by);

-- proposal_jobs -----------------------------------------------------------------------------------
ALTER TABLE proposal_jobs ALTER COLUMN created_by DROP NOT NULL, ALTER COLUMN created_by DROP DEFAULT;
UPDATE proposal_jobs j SET created_by = (SELECT u.id FROM users u WHERE u.id = j.created_by);
ALTER TABLE proposal_jobs ADD CONSTRAINT proposal_jobs_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_proposal_jobs_created_by ON proposal_jobs (created_by);

-- discovery_feedback ------------------------------------------------------------------------------
ALTER TABLE discovery_feedback ALTER COLUMN actor_id DROP NOT NULL;
UPDATE discovery_feedback f SET actor_id = (SELECT u.id FROM users u WHERE u.id = f.actor_id);
ALTER TABLE discovery_feedback ADD CONSTRAINT discovery_feedback_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_discovery_feedback_actor ON discovery_feedback (actor_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
