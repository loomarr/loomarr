-- +goose Up
-- Secret field names are safe routing metadata. Persisting them separately lets reads report
-- configured fields without opening the single encrypted credential envelope.
-- NULL marks a pre-migration row whose exact optional-field state is unknown. Reads conservatively
-- report the provider's secret fields as configured until an explicit update writes exact metadata.
ALTER TABLE notification_destinations ADD COLUMN credential_keys_json TEXT;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
