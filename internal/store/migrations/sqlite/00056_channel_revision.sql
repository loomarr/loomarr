-- +goose Up
-- Every channel-row mutation participates in one optimistic concurrency
-- protocol. Existing rows begin at revision 1; revision 0 remains an in-memory
-- create sentinel and is never persisted.
ALTER TABLE channels
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision >= 1);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
