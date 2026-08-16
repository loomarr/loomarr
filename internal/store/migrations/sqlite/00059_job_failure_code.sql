-- +goose Up
-- Public Proposal Job reads expose a bounded failure category, never the provider/model
-- diagnostic retained in last_error for operators. The default keeps rows written by an
-- older process readable during a rolling upgrade.
ALTER TABLE jobs ADD COLUMN failure_code TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
