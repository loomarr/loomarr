-- +goose Up
-- PostgreSQL mirror of the Browser Push subscription identity constraint (§11).
ALTER TABLE notification_destinations
    ADD COLUMN subscription_fingerprint TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_notification_destinations_web_push_subscription
    ON notification_destinations (owner_id, subscription_fingerprint)
    WHERE means = 'web_push' AND subscription_fingerprint <> '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
