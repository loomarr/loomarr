-- +goose Up
-- PostgreSQL mirror of SQLite 00108; the full rationale lives there.

UPDATE filler_clip_pipeline
SET disposition = CASE
  WHEN clip_hash IN (SELECT hash FROM clips WHERE is_composite = TRUE) THEN 'complete'
  ELSE 'ready'
END
WHERE disposition = 'filed';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
