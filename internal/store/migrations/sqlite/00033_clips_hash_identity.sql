-- +goose Up
-- V38c: clip identity becomes a CONTENT HASH, and the catalog is rebuilt around it (§10).
--
-- ⚠ **The catalog is DROPPED and recreated.** This is the third identity change (§15's migration
-- note records all three) and the same move `00006` and §9.1 made, for the same reason: `clips` is
-- a synced CACHE of what is on disk, so the next scan repopulates it. Recomputing ids in-place is
-- not an option — an id is a hash of the file's BYTES, so the migration would have to read every
-- file in the operator's library, and a migration that does I/O over a media library is not a
-- migration.
--
-- ⚠ **What is lost, stated plainly** (maintainer, 2026-08-02: accepted):
--   - TAGS are RECOVERABLE. V38c writes them to each clip's `.info.json`, and the next scan reads
--     them back. An install that has been tagging since V38c loses nothing.
--   - PLAY COUNTS and channel PINS are NOT. They live only here, and no scan can restore them.
--   - A PRE-V38c install has no sidecars at all, so its hand-typed tags are gone outright. That is
--     the case the in-app warning exists for — §10 requires the loss to be surfaced rather than
--     left to be noticed.
--
-- The schema is rebuilt rather than ALTERed because the PRIMARY KEY changes: `path` was the key
-- and is now ordinary data, while `hash` becomes the key. SQLite cannot re-key a table in place.

DROP TABLE IF EXISTS clips;

CREATE TABLE clips (
  -- The sparse content hash: 64 lowercase hex characters (see filler.ClipID).
  hash              TEXT PRIMARY KEY,
  -- Location under the clip folder — `a3/f9/<hash>.mp4`. ⚠ Data, not identity: two folders can
  -- hold the same bytes, and the point of hashing is that they are ONE clip.
  path              TEXT NOT NULL,
  tunarr_program_id TEXT,
  name              TEXT NOT NULL,
  kind              TEXT NOT NULL,
  era               INTEGER NOT NULL DEFAULT 0,
  audience          TEXT NOT NULL DEFAULT '',
  category          TEXT NOT NULL DEFAULT '',
  duration_ms       INTEGER NOT NULL,
  rating            TEXT NOT NULL DEFAULT '',
  source            TEXT NOT NULL DEFAULT '',
  ai_tagged         INTEGER NOT NULL DEFAULT 0,
  quality           TEXT NOT NULL DEFAULT '',
  license           TEXT NOT NULL DEFAULT '',
  thumbnail         TEXT NOT NULL DEFAULT '',
  play_count        INTEGER NOT NULL DEFAULT 0,
  last_played_at    INTEGER NOT NULL DEFAULT 0,
  suggested_era     INTEGER NOT NULL DEFAULT 0,
  removed_at        INTEGER NOT NULL DEFAULT 0,
  held              INTEGER NOT NULL DEFAULT 0,
  confidence        INTEGER NOT NULL DEFAULT 0,
  auto_filed        INTEGER NOT NULL DEFAULT 0,
  updated_at        INTEGER NOT NULL
);

-- The reads that run on every pod assembly (00028, 00031 carried these before the rebuild).
CREATE INDEX IF NOT EXISTS idx_clips_removed_at ON clips(removed_at);
CREATE INDEX IF NOT EXISTS idx_clips_held ON clips(held);
-- ⚠ Path is looked up by intake to answer "is this file already catalogued?" on every scan.
CREATE INDEX IF NOT EXISTS idx_clips_path ON clips(path);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
