-- +goose Up
-- §10 the clip lifecycle (V38): an ingested clip is HELD until it is filed.
--
-- Until now a clip had no lifecycle. The folder scan catalogued it and the tagger tagged it in
-- place, so everything Loomarr downloaded was playable the moment it landed — tagged or not,
-- right or wrong. `held` is the state before the catalog: recorded, but not matched into a pod,
-- not attached to a filler-list, and not counted as coverage.
--
-- 0 = filed (the catalog proper), 1 = held. ⚠ **The default is 0, and that is load-bearing.**
-- Every clip already in this table was catalogued under the old model and is playing on a
-- channel RIGHT NOW; defaulting to 1 would move the entire existing catalog out of matching and
-- silently empty every channel's filler pool on upgrade. This is the same class of mistake as
-- 00026's first draft, where a column default chosen for new rows would have switched off every
-- source that already existed.
--
-- ⚠ **Only INGESTED clips are held.** A file an operator hand-copies into FILLER_DIR is a
-- deliberate human act and is filed on sight (§10) — holding it would mean a clip you placed
-- yourself sits invisible until you approve it. The scan writes `held = 0`; the ingest path
-- writes `held = 1`. That asymmetry lives in the writers, not here.
--
-- The read side follows `removed_at` exactly (00028): filtered at the ONE chokepoint in
-- ListClips, with an explicit opt-in flag for the surface that needs to see held clips
-- (Incoming). Filtering at each call site is how one of them gets forgotten and a held clip
-- plays.
ALTER TABLE clips ADD COLUMN held INTEGER NOT NULL DEFAULT 0;

-- Matching reads filter on this on every pod assembly, so it is worth an index for the same
-- reason `removed_at` has one.
CREATE INDEX IF NOT EXISTS idx_clips_held ON clips(held);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
