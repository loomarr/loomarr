-- +goose Up
-- SQLite cannot widen CHECK constraints, so rebuild the linked tables transactionally.
DROP INDEX idx_notification_attempts_due;
DROP INDEX idx_notification_intents_terminal;
DROP INDEX idx_notification_intents_reference;

ALTER TABLE notification_delivery_attempts RENAME TO notification_delivery_attempts_old;
ALTER TABLE notification_intents RENAME TO notification_intents_old;

CREATE TABLE notification_intents (
    id                TEXT PRIMARY KEY,
    topic             TEXT NOT NULL CHECK (topic IN (
        'account_invitation', 'local_password_recovery',
        'proposal_submitted', 'proposal_approved', 'proposal_declined',
        'acquisition_available', 'acquisition_gave_up', 'channel_live', 'channel_degraded',
        'delivery_test'
    )),
    recipient_kind    TEXT NOT NULL CHECK (recipient_kind IN ('invitation', 'person', 'approvers', 'operators')),
    recipient_id      TEXT NOT NULL,
    reference_kind    TEXT NOT NULL CHECK (reference_kind IN ('invitation', 'recovery', 'proposal', 'title', 'channel', 'destination')),
    reference_id      TEXT NOT NULL,
    recipient_policy  TEXT NOT NULL CHECK (recipient_policy IN ('mandatory_account', 'configurable')),
    template_json     TEXT NOT NULL,
    idempotency_key   TEXT NOT NULL UNIQUE,
    created_at        BIGINT NOT NULL,
    terminal_at       BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE notification_delivery_attempts (
    id                    TEXT PRIMARY KEY,
    intent_id             TEXT NOT NULL REFERENCES notification_intents(id) ON DELETE CASCADE,
    means                 TEXT NOT NULL CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    )),
    destination_ref       TEXT NOT NULL,
    destination_redacted  TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (status IN ('queued', 'sending', 'delivered', 'failed', 'suppressed')),
    attempt_number        INTEGER NOT NULL CHECK (attempt_number BETWEEN 1 AND 5),
    available_at          BIGINT NOT NULL,
    lease_owner           TEXT NOT NULL DEFAULT '',
    lease_expires_at      BIGINT NOT NULL DEFAULT 0,
    started_at            BIGINT NOT NULL DEFAULT 0,
    finished_at           BIGINT NOT NULL DEFAULT 0,
    provider_message_id   TEXT NOT NULL DEFAULT '',
    failure_class         TEXT NOT NULL DEFAULT '' CHECK (failure_class IN ('', 'transient_pre_acceptance', 'permanent', 'ambiguous_acceptance', 'cancelled')),
    outcome_code          TEXT NOT NULL DEFAULT '' CHECK (outcome_code IN ('', 'delivery_disabled', 'destination_unavailable', 'preference_disabled', 'means_unavailable', 'recipient_rejected', 'configuration_invalid', 'transport_unavailable', 'acceptance_ambiguous', 'cancelled', 'worker_interrupted')),
    created_at            BIGINT NOT NULL,
    UNIQUE (intent_id, means, destination_ref, attempt_number),
    CHECK (
        (status = 'queued' AND lease_owner = '' AND lease_expires_at = 0 AND started_at = 0 AND finished_at = 0 AND provider_message_id = '' AND failure_class = '' AND outcome_code = '') OR
        (status = 'sending' AND lease_owner <> '' AND lease_expires_at > 0 AND started_at > 0 AND finished_at = 0 AND provider_message_id = '' AND failure_class = '' AND outcome_code = '') OR
        (status = 'delivered' AND lease_owner = '' AND lease_expires_at = 0 AND finished_at > 0 AND failure_class = '' AND outcome_code = '') OR
        (status = 'failed' AND lease_owner = '' AND lease_expires_at = 0 AND finished_at > 0 AND provider_message_id = '' AND failure_class <> '' AND outcome_code <> '') OR
        (status = 'suppressed' AND lease_owner = '' AND lease_expires_at = 0 AND finished_at > 0 AND provider_message_id = '' AND failure_class = '' AND outcome_code IN ('delivery_disabled', 'destination_unavailable', 'preference_disabled'))
    )
);

INSERT INTO notification_intents SELECT * FROM notification_intents_old;
INSERT INTO notification_delivery_attempts SELECT * FROM notification_delivery_attempts_old;

DROP TABLE notification_delivery_attempts_old;
DROP TABLE notification_intents_old;

CREATE INDEX idx_notification_attempts_due
    ON notification_delivery_attempts (available_at, id) WHERE status = 'queued';
CREATE INDEX idx_notification_intents_terminal
    ON notification_intents (terminal_at) WHERE terminal_at > 0;
CREATE INDEX idx_notification_intents_reference
    ON notification_intents (reference_kind, reference_id, created_at DESC, id DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
