-- +goose Up
-- The library scanner's notifications (#2151 decision 32): a media folder goes offline or comes back,
-- and an arr can no longer tell Loomarr about new files. They reference a folder or an arr.
ALTER TABLE notification_intents
    DROP CONSTRAINT notification_intents_topic_check,
    DROP CONSTRAINT notification_intents_reference_kind_check;

ALTER TABLE notification_intents
    ADD CONSTRAINT notification_intents_topic_check CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_declined',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded',
        'delivery_test',
        'library_folder_offline', 'library_folder_back', 'library_changes_degraded'
    )),
    ADD CONSTRAINT notification_intents_reference_kind_check CHECK (
        reference_kind IN ('invitation', 'recovery', 'proposal', 'title', 'channel', 'destination', 'library')
    );

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
