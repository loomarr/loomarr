-- +goose Up
-- Delivery tests are durable notification work, but are deliberately not product transitions.
ALTER TABLE notification_intents
    DROP CONSTRAINT notification_intents_topic_check,
    DROP CONSTRAINT notification_intents_reference_kind_check;

ALTER TABLE notification_intents
    ADD CONSTRAINT notification_intents_topic_check CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_declined',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded',
        'delivery_test'
    )),
    ADD CONSTRAINT notification_intents_reference_kind_check CHECK (
        reference_kind IN ('invitation', 'recovery', 'proposal', 'title', 'channel', 'destination')
    );

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
