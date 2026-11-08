-- +goose Up
-- V45a: the clip taxonomy (§10) — a multi-tag vocabulary over an operator-editable graph, replacing
-- the flat `category` string.
--
-- Two tables:
--
--   taxa       the taxonomy graph. One row per taxon: slug (stable id), label, parent (the IS-A edge
--              within an axis), axis (product|format|seasonal|audience-cue), and JSON arrays of
--              synonyms + retired aliases (the resolve index the tagger grounds against). This is the
--              SOURCE OF TRUTH; an operator may add/edit rows.
--
--   clip_tags  the denormalised clip↔taxon rows. A clip tagged `beer` gets THREE rows — beer(leaf=1),
--              alcohol(leaf=0), drinks(leaf=0) — so a curation query `WHERE taxon='food'` is one index
--              hit, no graph walk (pod assembly runs it per break per reconcile). `leaf` distinguishes
--              an asserted tag from a derived rollup: a re-tag replaces leaves, rollups are always
--              recomputed from the current graph by the reindex job (§10 V45a).
--
-- ⚠ The taxa table is SEEDED AT BOOT from taxonomy.SeedForest(), not here — one source of truth, so
-- the Go graph and the DB cannot drift. The seeder is idempotent and only writes when taxa is empty,
-- so it never clobbers an operator's edits. This migration creates the empty tables.
--
-- ⚠ `category` on `clips` is NOT dropped — it survives as a derived shadow (the primary product leaf)
-- so every existing reader keeps working during the migration. clip_tags is the new source for
-- curation; category is kept in step by the tagger.
--
-- Forward-only (§16). clip_tags is a synced cache of (clips × taxa) — rebuildable by re-tagging and
-- the reindex, so it carries no data a backup must protect beyond what regenerates.
CREATE TABLE taxa (
  slug        TEXT PRIMARY KEY,
  label       TEXT NOT NULL,
  parent      TEXT NOT NULL DEFAULT '',
  axis        TEXT NOT NULL,
  synonyms    TEXT NOT NULL DEFAULT '[]',   -- JSON array
  aliases     TEXT NOT NULL DEFAULT '[]',   -- JSON array (retired slugs)
  updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE clip_tags (
  clip_hash TEXT NOT NULL,
  taxon     TEXT NOT NULL,
  leaf      BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (clip_hash, taxon)
);

-- The curation read path: "clips tagged (a descendant of) X". Index on taxon so it is one hit.
CREATE INDEX idx_clip_tags_taxon ON clip_tags (taxon);
-- The reindex/read-a-clip's-tags path.
CREATE INDEX idx_clip_tags_clip ON clip_tags (clip_hash);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
