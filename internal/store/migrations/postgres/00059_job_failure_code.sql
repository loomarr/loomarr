-- +goose Up
-- Postgres mirror of sqlite 00059: separate the safe public failure category from the
-- private provider/model diagnostic kept in last_error.
ALTER TABLE jobs ADD COLUMN failure_code TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
