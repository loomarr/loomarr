-- +goose Up
-- Postgres mirror of the immutable accepted-cycle anchor. Existing beta rows
-- receive their last durable channel timestamp once.
ALTER TABLE channels
    ADD COLUMN playout_anchor BIGINT NOT NULL DEFAULT 0 CHECK (playout_anchor >= 0);
UPDATE channels SET playout_anchor = updated_at WHERE playout_anchor = 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
