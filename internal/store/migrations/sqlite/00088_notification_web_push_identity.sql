-- +goose Up
-- The protected Push endpoint remains in the credential envelope. Its one-way fingerprint is safe
-- routing metadata and makes a person's subscription unique under concurrent creates (§11).
ALTER TABLE notification_destinations
    ADD COLUMN subscription_fingerprint TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_notification_destinations_web_push_subscription
    ON notification_destinations (owner_id, subscription_fingerprint)
    WHERE means = 'web_push' AND subscription_fingerprint <> '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
