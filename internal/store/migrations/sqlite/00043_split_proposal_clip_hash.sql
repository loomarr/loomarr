-- +goose Up
-- V51a (§10): the split proposal keys on the compilation's HASH, not on its path.
--
-- ⚠ **This column never held what Confirm needed, and split confirm has been broken since the
-- V38c identity change because of it.** `Propose` wrote `clip.Path` (the shard path
-- `a3/f9/<hash>.mp4`); `Confirm` handed that same string to `GetClip`, which is `WHERE hash = ?`.
-- The lookup never matched, so every confirm returned "compilation … no longer in the catalog"
-- for a clip sitting in the catalog. An operator could open a 41-segment reel, edit the cut list,
-- and never commit it.
--
-- The NAME is the bug: `clip_path` described the value, `GetClip` needed the identity, and the
-- two silently diverged the moment identity moved off the path. Renaming the column is what stops
-- the next reader storing a path in it again.
--
-- ⚠ Pending rows are DELETED rather than converted. A path could be mapped back to a hash — it is
-- the basename — but these proposals were never confirmable, so converting one preserves a review
-- the operator has already been unable to action. Re-detection is one job tick, and the
-- compilation itself is untouched (nothing here is a clip). Nothing reachable is lost.
-- ⚠ **00042 is skipped on purpose — do not "fix" the gap.** It is owned by the in-flight
-- `feat/hevc-direct-play` branch (`00042_channel_broadcast_codec.sql`), which is not on main yet.
-- Two branches numbering the next migration the same way is the concrete form of the conflict
-- CLAUDE.md's worktree table warns about; goose applies in version order and records what it has
-- run, so a gap costs nothing and a duplicate version costs a migration.
DELETE FROM filler_split_proposals;

ALTER TABLE filler_split_proposals RENAME COLUMN clip_path TO clip_hash;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
