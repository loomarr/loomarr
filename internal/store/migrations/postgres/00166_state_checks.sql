-- +goose Up
-- Schema naming (#2323): the core state columns accept only the states the code knows, so a
-- misspelt state fails at the write instead of parking a row where no query looks.
ALTER TABLE titles ADD CONSTRAINT titles_state_check
    CHECK (state IN ('wanted', 'requested', 'downloading', 'available', 'unavailable'));
ALTER TABLE proposals ADD CONSTRAINT proposals_status_check
    CHECK (status IN ('submitted', 'approved', 'denied', 'superseded'));
ALTER TABLE proposal_jobs ADD CONSTRAINT proposal_jobs_status_check
    CHECK (status IN ('queued', 'running', 'done', 'failed'));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
