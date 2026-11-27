-- +goose Up
-- #4414. Postgres mirror of sqlite 00250; the full reasoning lives there.

DROP TABLE filler_split_shadow_decisions;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
