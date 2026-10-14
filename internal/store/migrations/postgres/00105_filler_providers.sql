-- +goose Up
-- V51c extension. PostgreSQL mirror of SQLite 00105; the full reasoning lives there.

CREATE TABLE filler_providers (
  kind    TEXT PRIMARY KEY CHECK (kind IN ('archive', 'youtube')),
  enabled BOOLEAN NOT NULL DEFAULT TRUE
);

INSERT INTO filler_providers (kind, enabled) VALUES
  ('archive', TRUE),
  ('youtube', TRUE);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
