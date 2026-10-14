-- +goose Up
-- Home's New this week (#1663): when a title arrived, and when a channel was created.
--
-- titles.available_at is stamped by the provisioning state machine when the library confirms an
-- in-flight title (provision.Apply, LibraryConfirmed). Titles a channel picked from the library
-- are written straight to available and keep 0: they never arrived, they were already there.
-- Backfill: an available title that went through a request last changed state when it arrived,
-- so its updated_at is the arrival. Anything else stays 0 (unknown, never "new").
ALTER TABLE titles ADD COLUMN available_at BIGINT NOT NULL DEFAULT 0;
UPDATE titles SET available_at = updated_at WHERE state = 'available' AND requested_at > 0;
CREATE INDEX idx_titles_available_at ON titles (available_at);

-- channels.created_at is stamped on insert and never updated. Backfill a proposal-born channel
-- from its job's first approval; a hand-made one stays 0 (unknown, never "new").
ALTER TABLE channels ADD COLUMN created_at BIGINT NOT NULL DEFAULT 0;
UPDATE channels SET created_at = COALESCE((
    SELECT MIN(p.approved_at) FROM proposals p
    WHERE p.job_id = channels.intent_ref AND p.status = 'approved' AND p.approved_at > 0), 0)
WHERE intent_ref <> '';

-- +goose Down
SELECT 1;
