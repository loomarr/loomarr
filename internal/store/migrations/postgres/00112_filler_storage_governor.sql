-- +goose Up
-- PostgreSQL mirror of SQLite 00112; the full rationale lives there.

INSERT INTO settings (key, value, updated_at, updated_by, env_override)
SELECT 'filler.storage.library_budget_gb', value, updated_at, updated_by, FALSE
FROM settings
WHERE key = 'filler.fetch.max_disk_gb'
  AND env_override = FALSE
  AND value ~ '^[0-9]+$'
  AND value::BIGINT > 0
ON CONFLICT (key) DO NOTHING;

DELETE FROM settings WHERE key = 'filler.fetch.max_disk_gb';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
