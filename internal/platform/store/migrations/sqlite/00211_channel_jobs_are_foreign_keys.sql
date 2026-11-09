-- +goose Up
-- Schema to a 10 (#2793, item 1): a channel and the suggestion jobs about it reference each other
-- by foreign key.
--
--   - channels.from_request says the channel was made from a member's request. intent_ref used to
--     say it by being non-empty, but the job it names is purged once settled, so it cannot carry
--     that for the channel's life;
--   - channels.intent_ref, the job a channel was made from, references proposal_jobs and is
--     cleared when that job is purged (SET NULL);
--   - proposal_jobs.channel_id, the channel a refine or re-curation job changes, references
--     channels and goes with it (CASCADE). A suggestion whose job is gone is refused as "channel
--     gone": the purge keeps every job a submitted suggestion needs, so only a deleted channel
--     takes one away.
--
-- Both links used '' for "none", which no row has as its id, so they become NULL, and a link
-- naming no row is cleared; a job naming a channel that is gone is dropped, as deleting the
-- channel now does.
--
-- The two keys make a cycle, so intent_ref is checked at commit (DEFERRABLE INITIALLY DEFERRED):
-- copying the database to Postgres in one transaction can then put channels before proposal_jobs.
--
-- SQLite adds a foreign key to an existing column by adding a keyed column, copying the values
-- across, and dropping the old one under its name: no table is rebuilt.

-- proposal_jobs.channel_id ----------------------------------------------------------------------
DELETE FROM proposal_jobs
WHERE channel_id <> '' AND NOT EXISTS (SELECT 1 FROM channels c WHERE c.id = proposal_jobs.channel_id);
ALTER TABLE proposal_jobs ADD COLUMN channel_ref TEXT REFERENCES channels(id) ON DELETE CASCADE;
UPDATE proposal_jobs SET channel_ref = channel_id WHERE channel_id <> '';
DROP INDEX idx_proposal_jobs_channel;
ALTER TABLE proposal_jobs DROP COLUMN channel_id;
ALTER TABLE proposal_jobs RENAME COLUMN channel_ref TO channel_id;
CREATE INDEX idx_proposal_jobs_channel ON proposal_jobs (channel_id);

-- channels.from_request and channels.intent_ref ----------------------------------------------------
ALTER TABLE channels ADD COLUMN from_request BOOLEAN NOT NULL DEFAULT FALSE CHECK (from_request IN (0, 1));
UPDATE channels SET from_request = TRUE WHERE intent_ref <> '';
ALTER TABLE channels ADD COLUMN intent_job TEXT
    REFERENCES proposal_jobs(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED;
UPDATE channels SET intent_job = intent_ref
WHERE EXISTS (SELECT 1 FROM proposal_jobs j WHERE j.id = channels.intent_ref);
DROP INDEX idx_channels_intent_ref;
ALTER TABLE channels DROP COLUMN intent_ref;
ALTER TABLE channels RENAME COLUMN intent_job TO intent_ref;
CREATE UNIQUE INDEX idx_channels_intent_ref ON channels (intent_ref);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
