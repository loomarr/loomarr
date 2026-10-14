-- +goose Up
-- V28: a clip's thumbnail, and an honest record of it having AIRED.
--
-- `thumbnail` is a path RELATIVE to the thumbnail cache directory, mirroring how `path` is
-- relative to FILLER_DIR (see 00013) and for the same reason: the mount differs between host
-- and container, so an absolute path would invalidate every row the first time someone moves
-- it. Empty = not generated yet, which renders as no image rather than a broken one.
--
-- The bytes are NOT stored here. Channel icons (00012) do store bytes, but a channel has one
-- icon and an install has thousands of clips — putting hundreds of MB of REGENERABLE images
-- into a table that rides the §16 backup would bloat every backup and every V11 migration for
-- data that ffmpeg can rebuild from the source file. `clips` is already a synced cache; its
-- thumbnails share that lifecycle.
--
-- ⚠ `play_count` / `last_played_at` are written from PLAYOUT, never from pod assembly.
-- Assembly is the tempting place and it is wrong twice over: `filler.Assemble` takes a `used`
-- map for de-duplication WITHIN one pod (adapter.go passes a fresh empty one every call, so it
-- has no memory), and pods are re-assembled on every reconcile sweep — so counting there would
-- inflate without bound and would count SCHEDULED, not AIRED.
--
-- Consequence, stated so nobody reads a wrong number: only internal playout can report this.
-- A Tunarr-backed channel airs its filler through Tunarr, which never tells us, so those clips
-- stay at 0. The UI must distinguish "never played" from "not counted here" — see the DTO.
--
-- Safe as plain ADD COLUMNs: `clips` is a synced cache (00013), so the worst case is one sync
-- cycle with blank thumbnails and counters starting from zero.
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN thumbnail TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN play_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE clips ADD COLUMN last_played_at BIGINT NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
