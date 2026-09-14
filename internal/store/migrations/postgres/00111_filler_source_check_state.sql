-- +goose Up
-- PostgreSQL mirror of SQLite 00111; the full rationale lives there.

ALTER TABLE filler_sources ADD COLUMN check_failure_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_sources ADD COLUMN check_retry_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE filler_sources ADD COLUMN check_lease_until BIGINT NOT NULL DEFAULT 0;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
