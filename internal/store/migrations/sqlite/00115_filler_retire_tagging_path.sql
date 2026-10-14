-- +goose Up
-- #1251: text classification moved out of readiness and into versioned progressive enrichment.
-- Resume an interrupted legacy tag rung at vision, remove its obsolete ladder record everywhere,
-- and discard the old second enable switch. The household AI selection now owns availability.

UPDATE filler_clip_pipeline
SET stage = 'vision', status = 'queued', progress = 0, attempts = 0, force_run = 0,
    next_run = 0, updated_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE stage = 'tag' AND disposition = 'running';

UPDATE filler_clip_pipeline
SET stages_json = (
  SELECT COALESCE(json_group_array(json(entry.value)), '[]')
  FROM json_each(filler_clip_pipeline.stages_json) AS entry
  WHERE json_extract(entry.value, '$.stage') <> 'tag'
)
WHERE json_valid(stages_json)
  AND EXISTS (
    SELECT 1 FROM json_each(filler_clip_pipeline.stages_json) AS entry
    WHERE json_extract(entry.value, '$.stage') = 'tag'
  );

DELETE FROM settings WHERE key = 'filler.ai_tagging';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
