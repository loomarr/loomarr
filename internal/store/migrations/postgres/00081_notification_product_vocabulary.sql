-- +goose Up
-- PostgreSQL can widen the notification CHECK constraints in place while preserving all rows.
ALTER TABLE notification_intents
    DROP CONSTRAINT notification_intents_topic_check,
    DROP CONSTRAINT notification_intents_recipient_kind_check,
    DROP CONSTRAINT notification_intents_reference_kind_check;

ALTER TABLE notification_intents
    ADD CONSTRAINT notification_intents_topic_check CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_declined',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded'
    )),
    ADD CONSTRAINT notification_intents_recipient_kind_check
        CHECK (recipient_kind IN ('invitation', 'person', 'approvers', 'operators')),
    ADD CONSTRAINT notification_intents_reference_kind_check
        CHECK (reference_kind IN ('invitation', 'recovery', 'proposal', 'title', 'channel'));

ALTER TABLE notification_delivery_attempts
    DROP CONSTRAINT notification_delivery_attempts_means_check;

ALTER TABLE notification_delivery_attempts
    ADD CONSTRAINT notification_delivery_attempts_means_check CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    ));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
