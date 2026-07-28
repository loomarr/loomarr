-- +goose Up
-- The unlock (config-design §3.1): a key an admin has explicitly taken back from
-- the environment, so the stored value wins even while the env var is still set.
--
-- ⚠ This MUST be durable, and that is the entire reason it is a column rather than
-- in-memory state. Env is re-read on every boot, so a session-scoped unlock would
-- resolve back to env on the next restart and silently discard whatever the operator
-- saved — the exact `LLM_MODEL` failure §3 records (the write succeeds, every read
-- still returns env, the value disappears on restart). Persisting it also puts the
-- claim in §16 backups, where it belongs: it is part of how this instance is
-- configured, not a UI preference.
--
-- Rides the settings KV rather than a side table: the flag is a property OF a
-- setting row, it is read on the same hot path as the value, and a second table
-- would need its own precedence rule for the case where the two disagree.
--
-- Forward-only (§16). Defaults to 0 so every pre-existing row keeps the old
-- contract exactly — env still wins for every key until someone unlocks one.

ALTER TABLE settings ADD COLUMN env_override BOOLEAN NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE settings DROP COLUMN env_override;
