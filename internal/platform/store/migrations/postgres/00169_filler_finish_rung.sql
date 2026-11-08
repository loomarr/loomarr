-- +goose Up
-- PostgreSQL mirror of SQLite 00169; the full rationale lives there.

UPDATE filler_clip_pipeline SET stage = 'finish' WHERE stage = 'score';

UPDATE filler_clip_pipeline AS pipeline
SET stages_json = renamed.stages_json
FROM (
  SELECT source.clip_hash,
         COALESCE(
           jsonb_agg(
             CASE WHEN entry.value->>'stage' = 'score'
               THEN jsonb_set(entry.value, '{stage}', '"finish"'::jsonb)
               ELSE entry.value
             END ORDER BY entry.ordinality),
           '[]'::jsonb
         )::text AS stages_json
  FROM filler_clip_pipeline AS source
  CROSS JOIN LATERAL jsonb_array_elements(source.stages_json::jsonb)
    WITH ORDINALITY AS entry(value, ordinality)
  WHERE source.stages_json::jsonb @> '[{"stage":"score"}]'::jsonb
  GROUP BY source.clip_hash
) AS renamed
WHERE pipeline.clip_hash = renamed.clip_hash;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
