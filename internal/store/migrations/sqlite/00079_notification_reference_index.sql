-- +goose Up
-- Invitation and recovery read models compose the newest delivery request by durable reference.
CREATE INDEX idx_notification_intents_reference
    ON notification_intents (reference_kind, reference_id, created_at DESC, id DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
