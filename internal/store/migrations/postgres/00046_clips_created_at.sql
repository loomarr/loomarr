-- +goose Up
-- V51d (§10). Postgres mirror of the sqlite 00045; the full reasoning lives there.
--
-- ⚠ In short: `updated_at` is bumped by every folder re-sync, so an "added" sort backed by it
-- would reorder the whole catalog after a routine scan. This column is written once, at insert,
-- and is omitted from `UpsertClip`'s DO UPDATE list for that reason.
--
-- ⚠ **BIGINT**, matching every other epoch column here (00033); sqlite spells the same 64-bit
-- value INTEGER. The type split between the dialects is per COLUMN — `internal/store/clips.go`
-- records that trap biting four times in one session.

ALTER TABLE clips ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT 0;

UPDATE clips SET created_at = updated_at WHERE created_at = 0;

-- +goose Down
-- Forward-only (§16).
ALTER TABLE clips DROP COLUMN IF EXISTS created_at;
