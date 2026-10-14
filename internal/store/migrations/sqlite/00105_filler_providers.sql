-- +goose Up
-- V51c extension: provider policy is independent of the child source registry.

CREATE TABLE filler_providers (
  kind    TEXT PRIMARY KEY CHECK (kind IN ('archive', 'youtube')),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1))
);

INSERT INTO filler_providers (kind, enabled) VALUES
  ('archive', 1),
  ('youtube', 1);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
