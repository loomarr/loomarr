-- +goose Up
-- V66 (§10): Postgres mirror of the SQLite legacy-filler quarantine. The full safety rationale
-- lives there; only BOOLEAN and epoch syntax differ here.
UPDATE clips
SET held = TRUE,
    auto_filed = FALSE,
    updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
WHERE removed_at = 0
  AND is_composite = FALSE;

UPDATE clips SET auto_filed = FALSE WHERE auto_filed = TRUE;

UPDATE filler_clip_pipeline
SET disposition = 'review',
    status = 'done',
    progress = 100,
    updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
WHERE disposition = 'filed'
  AND clip_hash IN (
    SELECT hash FROM clips
    WHERE held = TRUE AND removed_at = 0 AND is_composite = FALSE
  );

-- Forward-only (§16).

-- +goose Down
SELECT 1;
