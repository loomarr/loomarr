-- +goose Up
-- Source trust is a separate catalog-admission decision (§10 V57). Existing sources retain the
-- grounded auto-file behaviour they had before this policy became explicit.
ALTER TABLE filler_sources ADD COLUMN auto_admit BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
