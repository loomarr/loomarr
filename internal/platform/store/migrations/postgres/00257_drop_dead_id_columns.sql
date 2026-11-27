-- +goose Up
-- Schema to a 10 (#2793, item 1): every id column is a foreign key or a documented non-key. Two id
-- columns were neither, because nothing ever gave them a value to point at:
--
-- filler_inference_evaluations.run_id named "the batch run" for a per-run spend ceiling, but no
-- caller ever set a run, so the ceiling never applied and every row holds ''. The per-clip and
-- per-day ceilings are the ones in force. Its index goes with it.
--
-- interactive_operations.result_id held the split proposal a split detection produced, but nothing
-- reads it back: the result reaches the reel screen through the split event, and the one reader of
-- the table answers model pulls only.
DROP INDEX idx_filler_inference_evaluations_run_created;
ALTER TABLE filler_inference_evaluations DROP COLUMN run_id;
ALTER TABLE interactive_operations DROP COLUMN result_id;
DELETE FROM filler_inference_budget_guards WHERE scope LIKE 'run:%';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
