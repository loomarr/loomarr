-- +goose Up
-- V33: the persisted filler-sources registry, plus a clip's declared licence.
--
-- ⚠ **This REVERSES V28's read-model decision, deliberately and with a cost.** V28 made sources a
-- derived view precisely to avoid "a second source of truth needing a precedence rule against the
-- setting". V33 needs rows — a remote source has state (`last_fetched_at`, a licence, an operator's
-- decision to keep it) that no setting can hold — so the rule is now written down instead of
-- avoided: **the TABLE WINS, and `filler.dir` SEEDS it.**
--
-- The consequence, stated here because a future reader will hit it: once the local row exists,
-- editing `filler.dir` does not move the drop-folder by itself. `seeded_from` is what keeps that
-- from being silent — it records the setting value the row was created from, so the app can detect
-- a divergence and offer to adopt the new path rather than quietly ignoring the operator.

CREATE TABLE IF NOT EXISTS filler_sources (
  -- A stable id, not the URI: an operator re-pointing a source keeps its history.
  id           TEXT PRIMARY KEY,
  -- 'local' (a drop-folder) | 'archive' (an archive.org item/collection).
  kind         TEXT NOT NULL,
  -- The path (local) or the archive.org identifier/URL (remote).
  uri          TEXT NOT NULL,
  -- Operator-facing name; falls back to the uri when a source has none.
  label        TEXT NOT NULL DEFAULT '',
  -- The source's declared licence URL. ⚠ EMPTY MEANS UNKNOWN, never "public domain" —
  -- ~92% of archive.org items declare no licence at all (667 of 8362 measured in
  -- classic_tv_commercials, 2026-07-31), so absence is the normal case.
  license      TEXT NOT NULL DEFAULT '',
  -- ⚠ Only set on the SEEDED local row: the `filler.dir` value it was created from. Empty for
  -- every source an operator or discovery added. This is what makes "the setting changed but the
  -- table wins" visible instead of silent.
  seeded_from  TEXT NOT NULL DEFAULT '',
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
