-- +goose Up
-- Schema naming (#2323): foreign keys follow the policy in storage.md.
-- A crash report outlives the account that sent it: deleting a user clears `user_id` instead of
-- erasing the report. An airing belongs to its channel: deleting a channel deletes its airings, as
-- it already deletes the channel's feedback. Airings of channels deleted before this migration are
-- pruned first, because nothing could read them.
ALTER TABLE client_crash_reports DROP CONSTRAINT client_crash_reports_user_id_fkey;
ALTER TABLE client_crash_reports ADD CONSTRAINT client_crash_reports_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

DELETE FROM airings WHERE channel_id NOT IN (SELECT id FROM channels);
ALTER TABLE airings ADD CONSTRAINT airings_channel_id_fkey
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
