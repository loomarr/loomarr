-- +goose Up
-- V54 (§10). Postgres mirror of the sqlite 00050; the full reasoning lives there.
--
-- In short: a compilation that reached `review` at the split rung is UNREACHABLE, not waiting.
-- `ListPipelineWork` claims `disposition = 'running'` only, so no code path visits those rows
-- again. ~50 reels sat there because `AutoConfirmable` refused every one on a sub-floor segment
-- before their grounding was considered. The code fixes shipping alongside this cannot reach a row
-- nothing claims, so the rows are returned to the belt.
--
-- ⚠ A repair, not a re-decision: nothing is deleted, the proposals and their segments are
-- untouched, and a reel the gate refuses again parks with a real reason. A proposal detected before
-- this release still holds sub-floor segments and will re-park until an operator re-detects it —
-- deliberately not forced here, because deleting ~50 proposals unattended destroys review work.
UPDATE filler_clip_pipeline
   SET disposition = 'running',
       status      = 'queued',
       attempts    = 0,
       next_run    = 0
 WHERE stage = 'split'
   AND disposition = 'review';

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
