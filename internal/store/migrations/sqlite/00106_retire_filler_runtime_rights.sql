-- +goose Up
-- Runtime admission no longer interprets licensing. Preserve provider-declared licence text on
-- clip metadata, but discard the old current-use authority and every decision produced by the
-- superseded admission schema. Affected clips are held and re-enrolled from the beginning so an
-- old rights-dependent result cannot remain playable or silently survive as current truth.

CREATE TEMP TABLE retired_filler_decision_clips AS
SELECT DISTINCT clip_hash
FROM filler_admission_decisions
WHERE schema_version < 3;

UPDATE clips
SET held = TRUE,
    auto_filed = FALSE,
    updated_at = CAST(strftime('%s', 'now') AS INTEGER)
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

DROP TABLE retired_filler_decision_clips;
DROP TABLE filler_rights_heads;
DROP TABLE filler_rights_grants;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
