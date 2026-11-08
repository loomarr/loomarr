-- +goose Up
-- #2323: the terminal pipeline rung is renamed `score` → `finish`. It stopped computing a tagging
-- confidence (the number never decided anything once enrollment became the runtime authority) and
-- is now only the always-run anchor the ready commit validates against, which the Incoming tab
-- already called "Finishing". Stored rows move with it: the current stage, and every ladder record
-- in stages_json, so a published clip's ladder still ends with a completed terminal rung.

UPDATE filler_clip_pipeline SET stage = 'finish' WHERE stage = 'score';

UPDATE filler_clip_pipeline
SET stages_json = (
  SELECT COALESCE(json_group_array(
    CASE WHEN json_extract(entry.value, '$.stage') = 'score'
      THEN json_set(entry.value, '$.stage', 'finish')
      ELSE json(entry.value)
    END), '[]')
  FROM json_each(filler_clip_pipeline.stages_json) AS entry
)
WHERE json_valid(stages_json)
  AND EXISTS (
    SELECT 1 FROM json_each(filler_clip_pipeline.stages_json) AS entry
    WHERE json_extract(entry.value, '$.stage') = 'score'
  );

-- Forward-only (§16).

-- +goose Down
SELECT 1;
