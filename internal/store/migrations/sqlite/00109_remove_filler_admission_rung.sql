-- +goose Up
-- #1249 Wave 1: the runtime admission rung was retired in 00107, but its historical record stayed
-- inside stages_json. Remove that obsolete state so current ladders contain exactly the executable
-- stages. A row interrupted on the old rung resumes at score, the new terminal predecessor.

UPDATE filler_clip_pipeline
SET stage = 'score', status = 'queued', progress = 0, attempts = 0, force_run = 0,
    next_run = 0, updated_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE stage = 'admission' AND disposition = 'running';

UPDATE filler_clip_pipeline
SET stages_json = (
  SELECT COALESCE(json_group_array(json(entry.value)), '[]')
  FROM json_each(filler_clip_pipeline.stages_json) AS entry
  WHERE json_extract(entry.value, '$.stage') <> 'admission'
)
WHERE json_valid(stages_json)
  AND EXISTS (
    SELECT 1 FROM json_each(filler_clip_pipeline.stages_json) AS entry
    WHERE json_extract(entry.value, '$.stage') = 'admission'
  );

-- Forward-only (§16).

-- +goose Down
SELECT 1;
