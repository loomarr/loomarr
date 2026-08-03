-- +goose Up
-- V38c: clip identity becomes a CONTENT HASH, and the catalog is rebuilt around it (§10).
-- Postgres mirror of the sqlite 00033; the full reasoning lives there.
--
-- ⚠ The catalog is DROPPED and recreated — the same move 00006 and §9.1 made. `clips` is a synced
-- CACHE, so the next scan repopulates it; recomputing ids in place would mean hashing every file
-- in the operator's library from inside a migration.
--
-- ⚠ Tags are RECOVERABLE from each clip's `.info.json` (V38c writes them); play counts and channel
-- pins are NOT, and a pre-V38c install has no sidecars at all. §10 requires that loss to be
-- surfaced in the app rather than left to be noticed.
--
-- ⚠ **Three dialect differences from the sqlite file, all of which have bitten this session:**
-- BOOLEAN vs INTEGER for `ai_tagged`/`held`/`auto_filed`, BIGINT vs INTEGER for the epoch and
-- duration columns, and `TRUE/FALSE` vs `1/0` for their defaults. 00029 shipped a literal `1` into
-- a BOOLEAN column and took a hard 42804 at migrate time. The files read identically side by side;
-- only `test-pg` tells them apart.

DROP TABLE IF EXISTS clips;

CREATE TABLE clips (
  hash              TEXT PRIMARY KEY,
  path              TEXT NOT NULL,
  tunarr_program_id TEXT,
  name              TEXT NOT NULL,
  kind              TEXT NOT NULL,
  era               INTEGER NOT NULL DEFAULT 0,
  audience          TEXT NOT NULL DEFAULT '',
  category          TEXT NOT NULL DEFAULT '',
  duration_ms       BIGINT NOT NULL,
  rating            TEXT NOT NULL DEFAULT '',
  source            TEXT NOT NULL DEFAULT '',
  ai_tagged         BOOLEAN NOT NULL DEFAULT FALSE,
  quality           TEXT NOT NULL DEFAULT '',
  license           TEXT NOT NULL DEFAULT '',
  thumbnail         TEXT NOT NULL DEFAULT '',
  play_count        BIGINT NOT NULL DEFAULT 0,
  last_played_at    BIGINT NOT NULL DEFAULT 0,
  suggested_era     INTEGER NOT NULL DEFAULT 0,
  removed_at        BIGINT NOT NULL DEFAULT 0,
  held              BOOLEAN NOT NULL DEFAULT FALSE,
  confidence        INTEGER NOT NULL DEFAULT 0,
  auto_filed        BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at        BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_clips_removed_at ON clips(removed_at);
CREATE INDEX IF NOT EXISTS idx_clips_held ON clips(held);
CREATE INDEX IF NOT EXISTS idx_clips_path ON clips(path);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
