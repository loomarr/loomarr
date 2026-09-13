-- +goose Up
-- PostgreSQL mirror of SQLite 00107; the full rationale lives there.

ALTER TABLE clips DROP CONSTRAINT IF EXISTS clips_unclassified_held;
ALTER TABLE clips ADD COLUMN placement TEXT NOT NULL DEFAULT 'not_playable';

UPDATE clips
SET placement = CASE
  WHEN held = FALSE AND is_composite = FALSE AND kind IN ('bumper', 'station_id') THEN 'bookend'
  WHEN held = FALSE AND is_composite = FALSE AND kind IN ('commercial', 'psa', 'trailer', 'interstitial') THEN 'break_body'
  ELSE 'not_playable'
END;

CREATE TABLE filler_ready_events (
  id              TEXT PRIMARY KEY,
  clip_hash       TEXT NOT NULL,
  acquisition_id  TEXT NOT NULL DEFAULT '',
  enrollment_kind TEXT NOT NULL,
  enrollment_ref  TEXT NOT NULL,
  placement       TEXT NOT NULL,
  outcome         TEXT NOT NULL DEFAULT 'ready',
  created_at      BIGINT NOT NULL
);

CREATE INDEX idx_filler_ready_events_recent
  ON filler_ready_events(created_at DESC, id DESC);
CREATE UNIQUE INDEX idx_filler_ready_events_clip
  ON filler_ready_events(clip_hash);

UPDATE filler_clip_pipeline
SET stage = 'score', status = 'queued', progress = 0, disposition = 'running',
    reject_reason = '', reject_detail = '', attempts = 0, force_run = FALSE, next_run = 0,
    updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
WHERE disposition = 'review'
  AND clip_hash IN (
    SELECT hash FROM clips
    WHERE held = TRUE AND removed_at = 0 AND is_composite = FALSE
      AND (source <> '' OR hash IN (
        SELECT clip_hash FROM filler_clip_pipeline WHERE acquisition_id <> ''
      ))
  );

DELETE FROM filler_diagnostic_recovery_actions;
DELETE FROM filler_admission_actions;
DELETE FROM filler_admission_decision_inference_refs;
DELETE FROM filler_admission_decisions;

ALTER TABLE filler_sources DROP COLUMN auto_admit;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
