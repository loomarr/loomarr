-- +goose Up
-- PostgreSQL mirror of SQLite 00110; the full rationale lives there.

ALTER TABLE filler_sources ADD COLUMN last_checked_at BIGINT NOT NULL DEFAULT 0;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
