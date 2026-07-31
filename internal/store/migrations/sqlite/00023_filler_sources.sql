-- +goose Up
-- V33: the persisted filler-sources registry, plus a clip's declared licence.
--
-- ⚠ **This does NOT reverse V28's read-model decision — it sits beside it.** V28 made sources a
-- derived view to avoid "a second source of truth needing a precedence rule against the setting",
-- and that still holds: the drop-folder and the media-server library stay DERIVED from config, so
-- `filler.dir` remains the only thing that says where the folder is. There is no precedence rule
-- to write because the two never describe the same source.
--
-- This table holds REMOTE sources only — the specific archive.org collections an operator added,
-- which carry state no setting can express (a licence, a last-fetch time, the fact that someone
-- chose this one). On the Sources tab they nest under the existing `remote` row.

CREATE TABLE IF NOT EXISTS filler_sources (
  -- A stable id, not the URI: an operator re-pointing a source keeps its history.
  id           TEXT PRIMARY KEY,
  -- ⚠ Rows here are REMOTE sources only, and they nest UNDER the read-model's `remote` row
  -- rather than replacing it (maintainer decision, 2026-07-31). V28's three fixed rows —
  -- folder / library / remote — describe CONFIGURATION and stay derived: they answer "you could
  -- set up a library but have not", which a table of things-that-exist cannot express. This
  -- table answers a different question: which specific remote collections were added.
  --
  -- Keeping them separate is what stops one source appearing twice on the Sources tab, and it
  -- is why `kind` here does NOT reuse the DTO's folder/library/remote vocabulary.
  kind         TEXT NOT NULL,  -- 'archive' today; the discriminator exists for the next source type
  -- The path (local) or the archive.org identifier/URL (remote).
  uri          TEXT NOT NULL,
  -- Operator-facing name; falls back to the uri when a source has none.
  label        TEXT NOT NULL DEFAULT '',
  -- The source's declared licence URL. ⚠ EMPTY MEANS UNKNOWN, never "public domain" —
  -- ~92% of archive.org items declare no licence at all (667 of 8362 measured in
  -- classic_tv_commercials, 2026-07-31), so absence is the normal case.
  license      TEXT NOT NULL DEFAULT '',
  -- Epoch seconds; 0 = never fetched.
  last_fetched_at INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL
);

-- Listing is the only query, ordered for a stable UI.
CREATE INDEX IF NOT EXISTS idx_filler_sources_kind ON filler_sources(kind, created_at);

-- A clip's licence, carried from its source's info-JSON sidecar (filler.SidecarLicense).
-- Same empty-means-unknown rule as above.
ALTER TABLE clips ADD COLUMN license TEXT NOT NULL DEFAULT '';

-- Safe as a plain ADD COLUMN: `clips` is a synced CACHE (see 00013), so the worst case is one
-- sync cycle with blank licences until the next scan re-reads the sidecars.
--
-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
