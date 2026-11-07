-- +goose Up
-- The daily Needs you notice (#2151 decision 32): at most once a day, the operators hear how many
-- titles Loomarr can't tell what they are, when any are new since the last notice.
--
-- `library_items.needs_noticed` marks a title a notice already counted as new, so a restart neither
-- repeats a notice nor loses one.
ALTER TABLE library_items ADD COLUMN needs_noticed INTEGER NOT NULL DEFAULT 0;

ALTER TABLE notification_intents DROP CONSTRAINT notification_intents_topic_check;

ALTER TABLE notification_intents
    ADD CONSTRAINT notification_intents_topic_check CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_declined',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded',
        'delivery_test',
        'library_folder_offline', 'library_folder_back', 'library_changes_degraded',
        'library_needs_you'
    ));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
