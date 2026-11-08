-- +goose Up
-- Schema naming (#2323): a Proposal is approved or denied, and the column that records who decided
-- is `decided_by`, not `approved_by` (a denial wrote its denier there). The notification topic says
-- the same word as the status it reports: `proposal_denied`, not `proposal_declined`. Every stored
-- intent and every destination's chosen topics move to the new name.
ALTER TABLE proposals RENAME COLUMN approved_by TO decided_by;

UPDATE notification_destinations
   SET topics_json = REPLACE(topics_json, '"proposal_declined"', '"proposal_denied"')
 WHERE topics_json LIKE '%"proposal_declined"%';

ALTER TABLE notification_intents DROP CONSTRAINT notification_intents_topic_check;

UPDATE notification_intents SET topic = 'proposal_denied' WHERE topic = 'proposal_declined';

ALTER TABLE notification_intents
    ADD CONSTRAINT notification_intents_topic_check CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_denied',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded',
        'delivery_test',
        'library_folder_offline', 'library_folder_back', 'library_changes_degraded',
        'library_needs_you'
    ));

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
