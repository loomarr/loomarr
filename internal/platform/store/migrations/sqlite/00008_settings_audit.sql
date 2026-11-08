-- +goose Up
-- Settings audit columns (config-design §3): the settings KV grows `updated_at`
-- and `updated_by` so the UI can show "changed by Matt · 2d ago" per field, same
-- spirit as `approved_by` on the approval gate. env/migration writes leave
-- `updated_by` NULL (they have no human author).
--
-- Forward-only (§16). The second real ALTER TABLE after 00007's policy_json —
-- the bare (key,value) KV from 00001 holds durable overrides (Emby token, LLM
-- selection, instance id), so it is ALTERed in place, never dropped. `updated_at`
-- defaults to 0 (unknown) for pre-existing rows; new writes stamp a real epoch.

ALTER TABLE settings ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE settings ADD COLUMN updated_by TEXT;

-- +goose Down
ALTER TABLE settings DROP COLUMN updated_by;
ALTER TABLE settings DROP COLUMN updated_at;
