-- +goose Up
-- V51b (§10). Postgres mirror of the sqlite 00044; the full reasoning lives there.
--
-- ⚠ In short: pipeline state is a SIBLING of `clips`, not columns on it, because `clips` is a
-- synced cache that has been dropped and recreated twice — and this table records that Whisper
-- and a paid vision call have ALREADY been spent on a clip.
--
-- ⚠ **BIGINT for every epoch column**, matching the rest of this schema (00033 makes
-- `last_played_at`, `removed_at`, `updated_at` BIGINT; sqlite's INTEGER is already 64-bit). The
-- type split between the dialects is per COLUMN, and `internal/store/clips.go` records it biting
-- four times in one session.
--
-- ⚠ **No BOOLEAN columns anywhere here, deliberately.** Every state is a text enum, which stays
-- clear of the `1`-vs-`TRUE` 42804 class that 00029 hit and that `UpsertClip`'s bind list still
-- carries a warning about.

CREATE TABLE IF NOT EXISTS filler_clip_pipeline (
  clip_hash     TEXT PRIMARY KEY,
  stage         TEXT NOT NULL DEFAULT 'probe',
  status        TEXT NOT NULL DEFAULT 'queued',
  progress      INTEGER NOT NULL DEFAULT 0,
  disposition   TEXT NOT NULL DEFAULT 'running',
  reject_reason TEXT NOT NULL DEFAULT '',
  reject_detail TEXT NOT NULL DEFAULT '',
  attempts      INTEGER NOT NULL DEFAULT 0,
  next_run      BIGINT NOT NULL DEFAULT 0,
  stages_json   TEXT NOT NULL DEFAULT '[]',
  enrolled_at   BIGINT NOT NULL,
  updated_at    BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_filler_clip_pipeline_work
  ON filler_clip_pipeline(disposition, next_run);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
