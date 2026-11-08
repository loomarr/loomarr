-- +goose Up
-- PostgreSQL mirror of the provider-neutral delivery-summary lookup (§11).
CREATE INDEX idx_notification_intents_reference
    ON notification_intents (reference_kind, reference_id, created_at DESC, id DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
