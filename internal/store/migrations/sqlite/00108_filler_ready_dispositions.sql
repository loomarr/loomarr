-- +goose Up
-- #1249 Wave 1: `filed` overloaded playable clips and completed composite containers. Migrate the
-- beta state once into truthful terminal outcomes: Ready is playable; complete is a processed,
-- structurally non-playable container.

UPDATE filler_clip_pipeline
SET disposition = CASE
  WHEN clip_hash IN (SELECT hash FROM clips WHERE is_composite = 1) THEN 'complete'
  ELSE 'ready'
END
WHERE disposition = 'filed';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
