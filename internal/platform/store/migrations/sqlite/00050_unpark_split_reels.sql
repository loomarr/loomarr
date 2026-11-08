-- +goose Up
-- V54 (§10): return compilations parked at `split` to the belt.
--
-- ⚠ **These rows are unreachable, not merely waiting.** `VerdictReview` writes
-- `disposition='review'`, and `ListPipelineWork` claims `WHERE disposition = 'running'` only. A
-- reel that reached review at the split rung is therefore visited by no code path, ever again —
-- not by the cron, not by a manual run-now. Measured on the maintainer's catalog: ~50 compilations
-- in exactly this state, none ever auto-confirmed.
--
-- They got there because the gate could not pass. `AutoConfirmable` returns on the FIRST failing
-- segment and `RejectTooShort` sat above the grounding checks, so any reel containing a sub-floor
-- fragment — which is every real commercial compilation, measured at 39 of 82 segments on one
-- archive.org reel — was refused before its grounding was even considered. The two code fixes
-- (the floor moved to detection; the vision budget became a per-pass rate) make freshly-detected
-- reels work, but they cannot reach a row nothing claims.
--
-- ⚠ **A repair, not a re-decision.** The disposition was written by a gate that structurally could
-- not pass; putting the row back where the pipeline can see it restores the state it would have
-- been in had the gate worked. Nothing is deleted: the proposal, its segments and any grounding
-- already stamped on them are untouched, and a reel the gate refuses again returns to review
-- carrying a REAL reason this time.
--
-- ⚠ It can re-queue a reel an operator deliberately left alone, and that is a recorded decision
-- rather than an oversight. Two facts make it safe: no persisted operator edit can be lost (there
-- is no PATCH route — the review editor holds edits client-side until confirm), and auto-confirm
-- only fires when the strict all-or-nothing gate passes, which is the outcome the operator was
-- waiting for. A reel they wanted to cut by hand is still theirs to cut.
--
-- ⚠ **Honest limitation.** A proposal DETECTED before this release still holds sub-floor segments,
-- because the floor now applies at detection and this migration re-runs no detection. Such a reel
-- will re-gate, hit `RejectTooShort`, and park again — correctly, and this time with a reason on
-- the ladder. The remedy is an operator re-detect (`POST /v1/filler/split`), which replaces the
-- proposal. This migration deliberately does NOT delete proposals to force that: doing it to ~50
-- reels unattended would destroy review work to save a click.
--
-- `next_run = 0` means "due now" — the same zero-is-due rule `ListPipelineWork`'s `next_run <= ?`
-- already relies on. Attempts are reset because the retries these rows spent were spent losing to
-- a gate that could not be won, and carrying them would exhaust the budget on the way to succeeding.
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
