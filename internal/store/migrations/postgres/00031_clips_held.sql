-- +goose Up
-- §10 the clip lifecycle (V38): an ingested clip is HELD until it is filed. Postgres mirror of
-- the sqlite 00031; the full reasoning lives there and in §10 ("The clip lifecycle").
--
-- The short version: `held` is the state before the catalog — recorded, but not matched into a
-- pod, not attached to a filler-list, not counted as coverage.
--
-- ⚠ **FALSE is the default and it is load-bearing.** Every clip already in this table was
-- catalogued under the old model and is playing on a channel right now; defaulting to held would
-- move the entire existing catalog out of matching and silently empty every channel's filler
-- pool on upgrade — the same class as 00026's first draft, where a default chosen for new rows
-- would have switched off every source that already existed.
--
-- ⚠ BOOLEAN here where sqlite has INTEGER, like `auto_filed` in 00030. V37's 00029 shipped a
-- literal `1` into a Postgres BOOLEAN and took a hard 42804 at migrate time; the files read
-- identically side by side and only `test-pg` tells them apart.
ALTER TABLE clips ADD COLUMN held BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_clips_held ON clips(held);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
