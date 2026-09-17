-- +goose Up
-- #1240 replaces the folder-size fetch ceiling with the shared host-aware storage governor.
-- Preserve only a valid stored operator choice; an old environment override is not a durable
-- value and must be renamed by the operator rather than silently pinning the new key.

INSERT INTO settings (key, value, updated_at, updated_by, env_override)
SELECT 'filler.storage.library_budget_gb', value, updated_at, updated_by, 0
FROM settings
WHERE key = 'filler.fetch.max_disk_gb'
  AND env_override = 0
  AND value <> ''
  AND value NOT GLOB '*[^0-9]*'
  AND CAST(value AS INTEGER) > 0
ON CONFLICT (key) DO NOTHING;

DELETE FROM settings WHERE key = 'filler.fetch.max_disk_gb';

-- Forward-only (§16): the obsolete key has no runtime reader.

-- +goose Down
SELECT 1;
