-- +goose Up
-- PostgreSQL mirror of SQLite 00171; the full rationale lives there.
ALTER TABLE clips RENAME COLUMN kind TO role;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
