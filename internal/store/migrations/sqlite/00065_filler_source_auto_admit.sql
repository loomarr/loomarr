-- +goose Up
-- Source trust is a separate catalog-admission decision (§10 V57). Existing sources retain the
-- grounded auto-file behaviour they had before this policy became explicit.
ALTER TABLE filler_sources ADD COLUMN auto_admit INTEGER NOT NULL DEFAULT 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
