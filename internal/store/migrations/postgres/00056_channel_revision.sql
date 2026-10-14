-- +goose Up
-- Postgres mirror of the channel optimistic-concurrency token. A constant
-- default backfills every durable row without inventing an ordering among its
-- pre-migration writes.
ALTER TABLE channels
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
