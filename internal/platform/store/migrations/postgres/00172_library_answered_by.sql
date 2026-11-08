-- +goose Up
-- PostgreSQL mirror of SQLite 00172; the full rationale lives there.

ALTER TABLE library_answers ADD COLUMN answered_by TEXT NOT NULL DEFAULT 'you';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
