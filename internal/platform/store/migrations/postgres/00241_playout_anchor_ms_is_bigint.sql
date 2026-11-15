-- +goose Up
-- #4140: 00240 added `channels.playout_anchor_ms` as INTEGER. Every SQLite INTEGER is 64 bits, so
-- its Postgres twin is BIGINT (schemadoc.Parity); the column holds 0–999 either way.
ALTER TABLE channels ALTER COLUMN playout_anchor_ms TYPE BIGINT;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
