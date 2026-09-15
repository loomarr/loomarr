-- +goose Up
-- #1249 Wave 1: source/item enrollment is the household-use authority and terminal readiness is
-- one atomic runtime path. Classification remains metadata; it no longer keeps unknown-role clips
-- behind the obsolete admission review gate.

DROP TRIGGER IF EXISTS clips_unclassified_held_insert;
DROP TRIGGER IF EXISTS clips_unclassified_held_update;

ALTER TABLE clips ADD COLUMN placement TEXT NOT NULL DEFAULT 'not_playable';

UPDATE clips
SET placement = CASE
  WHEN held = 0 AND is_composite = 0 AND kind IN ('bumper', 'station_id') THEN 'bookend'
  WHEN held = 0 AND is_composite = 0 AND kind IN ('commercial', 'psa', 'trailer', 'interstitial') THEN 'break_body'
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
  created_at      INTEGER NOT NULL
);

CREATE INDEX idx_filler_ready_events_recent
  ON filler_ready_events(created_at DESC, id DESC);
CREATE UNIQUE INDEX idx_filler_ready_events_clip
  ON filler_ready_events(clip_hash);

-- Rows parked solely at the superseded admission/score review boundary get exactly one new chance
-- at the cheap final rung. Objective rejects, composites, removed clips, and active work are left
-- untouched.
UPDATE filler_clip_pipeline
SET stage = 'score', status = 'queued', progress = 0, disposition = 'running',
    reject_reason = '', reject_detail = '', attempts = 0, force_run = 0, next_run = 0,
    updated_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE disposition = 'review'
  AND clip_hash IN (
    SELECT hash FROM clips
    WHERE held = 1 AND removed_at = 0 AND is_composite = 0
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
