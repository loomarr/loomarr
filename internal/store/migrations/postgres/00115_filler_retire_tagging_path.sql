-- +goose Up
-- PostgreSQL mirror of SQLite 00115; the full rationale lives there.

UPDATE filler_clip_pipeline
SET stage = 'vision', status = 'queued', progress = 0, attempts = 0, force_run = FALSE,
    next_run = 0, updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
WHERE stage = 'tag' AND disposition = 'running';

UPDATE filler_clip_pipeline AS pipeline
SET stages_json = filtered.stages_json
FROM (
  SELECT source.clip_hash,
         COALESCE(
           jsonb_agg(entry.value ORDER BY entry.ordinality)
             FILTER (WHERE entry.value->>'stage' <> 'tag'),
           '[]'::jsonb
         )::text AS stages_json
  FROM filler_clip_pipeline AS source
  CROSS JOIN LATERAL jsonb_array_elements(source.stages_json::jsonb)
    WITH ORDINALITY AS entry(value, ordinality)
  GROUP BY source.clip_hash
) AS filtered
WHERE pipeline.clip_hash = filtered.clip_hash
  AND pipeline.stages_json::jsonb @> '[{"stage":"tag"}]'::jsonb;

DELETE FROM settings WHERE key = 'filler.ai_tagging';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
