-- +goose Up
-- V51b (§10): the per-clip ingest PIPELINE state.
--
-- ⚠ **A SIBLING TABLE, not columns on `clips`, and this is the load-bearing choice.** `clips` is
-- a synced CACHE of the drop-folder and has been DROPPED AND RECREATED twice — 00006 and 00033,
-- whose comment enumerates what that cost ("PLAY COUNTS and channel PINS are NOT recoverable").
-- Pipeline state records that we ALREADY SPENT ~341s of Whisper (§10 V40) and a paid vision call
-- (§10 V44) on a clip. Held in the cache, the next identity change re-runs every one of them
-- silently and re-spends the money. Held here, a rebuilt `clips` re-syncs from disk and the
-- pipeline correctly sees the work as done.
--
-- ⚠ It is also the only way to keep the single-writer discipline STRUCTURAL. `UpsertClip` omits
-- held/confidence/language/transcript/brand/visible_text/vision_tagged/is_composite/parent_hash
-- from its DO UPDATE by hand, and its own comment names "a future edit to one that forgot the
-- other" as the silent failure. A table the folder scan never touches cannot be forgotten.
--
-- ⚠ NO foreign key to `clips`. `DeleteClipsNotIn` is a bulk prune that races the scan, and an FK
-- would make one stale row fail the whole pass. The prune deletes orphans here in the same call.

CREATE TABLE IF NOT EXISTS filler_clip_pipeline (
  -- The clip's identity (§10 V38c) — the hash, never the path.
  clip_hash     TEXT PRIMARY KEY,
  -- The stage the clip is AT: probe|transcode|split|language|transcribe|tag|vision|score.
  stage         TEXT NOT NULL DEFAULT 'probe',
  -- That stage's status: queued|running|done|failed|skipped.
  --
  -- ⚠ Three non-terminal-looking values with DIFFERENT retry meanings, and conflating them is the
  -- bug this vocabulary exists to prevent. `failed` retries with backoff (a backend failure says
  -- nothing about the clip — the rule languagejob.go and transcribejob.go both state). `skipped`
  -- is RE-EVALUATED every pass but never re-run, so switching `filler.vision.enabled` on picks up
  -- clips that already went past that rung. `done` is never redone.
  status        TEXT NOT NULL DEFAULT 'queued',
  -- 0-100 within the CURRENT stage. Best-effort and deliberately throttled (§10): only the
  -- transcode stage has a real percentage (ffmpeg -progress); the rest report step boundaries.
  progress      INTEGER NOT NULL DEFAULT 0,
  -- running|review|filed|rejected — the clip-level outcome. `review` and `rejected` are the two
  -- an operator acts on; `filed` and `rejected` are terminal until a Rewind.
  disposition   TEXT NOT NULL DEFAULT 'running',
  -- A stable REJECT CODE (filler.RejectReason), '' when not rejected.
  --
  -- ⚠ The CODE, not prose. It is matched on by the UI, and §10's Incoming section is explicit
  -- that a reason is "derived from real state and never generated prose".
  reject_reason TEXT NOT NULL DEFAULT '',
  -- The MEASURED fact behind the code — "8.2s; floor is 10s", "detected es, expected en". This is
  -- what makes a reject arguable rather than an assertion.
  reject_detail TEXT NOT NULL DEFAULT '',
  -- Attempts of the CURRENT stage; reset on advance. Drives the backoff below.
  attempts      INTEGER NOT NULL DEFAULT 0,
  -- Epoch seconds this row is next eligible; 0 = now.
  --
  -- ⚠ The retry/backoff the cron jobs never had. `Work` always returned nil, so a failure simply
  -- waited for the next tick and a permanently-broken clip was retried at full cost forever.
  next_run      INTEGER NOT NULL DEFAULT 0,
  -- The finished ladder: [{"stage","status","note","attempts","at"}], in StageOrder.
  --
  -- ⚠ ONE JSON DOCUMENT, like `filler_split_proposals.segments_json` (00025) and
  -- `filler_pulls.plan_json` (00027), for the reason 00025 states: it is authored and read as a
  -- unit and never queried relationally. Stated cost: you cannot
  -- `WHERE stages_json->transcribe.status = 'failed'`. Accepted — the columns above answer every
  -- operator question ("what is stuck, and where").
  stages_json   TEXT NOT NULL DEFAULT '[]',
  enrolled_at   INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

-- The work-list read, which runs every pipeline pass.
CREATE INDEX IF NOT EXISTS idx_filler_clip_pipeline_work
  ON filler_clip_pipeline(disposition, next_run);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
