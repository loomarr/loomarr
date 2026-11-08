-- +goose Up
-- PostgreSQL mirror of SQLite 00106; the full rationale lives there.

CREATE TEMP TABLE retired_filler_decision_clips ON COMMIT DROP AS
SELECT DISTINCT clip_hash
FROM filler_admission_decisions
WHERE schema_version < 3;

UPDATE clips
SET held = TRUE,
    auto_filed = FALSE,
    updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
WHERE removed_at = 0
  AND hash IN (SELECT clip_hash FROM retired_filler_decision_clips);

DELETE FROM filler_clip_pipeline
WHERE clip_hash IN (SELECT clip_hash FROM retired_filler_decision_clips);

DELETE FROM filler_diagnostic_recovery_actions
WHERE decision_id IN (SELECT id FROM filler_admission_decisions WHERE schema_version < 3);
DELETE FROM filler_admission_actions
WHERE decision_id IN (SELECT id FROM filler_admission_decisions WHERE schema_version < 3);
DELETE FROM filler_admission_decision_inference_refs
WHERE decision_id IN (SELECT id FROM filler_admission_decisions WHERE schema_version < 3);
DELETE FROM filler_admission_decisions WHERE schema_version < 3;

DROP TABLE filler_rights_heads;
DROP TABLE filler_rights_grants;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
