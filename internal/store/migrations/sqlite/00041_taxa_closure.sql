-- +goose Up
-- V45a: the taxonomy CLOSURE table (§10) — the transitive-ancestor materialisation that makes the
-- rollup reindex a flat, dialect-neutral bulk join instead of an app-side per-clip loop.
--
-- `taxa` is an ADJACENCY LIST (`parent` points ONE level up). Deriving a clip's rollups from that
-- alone needs a recursive walk (`beer → alcohol → drinks`), which on the hot rebuild path would mean
-- either a per-clip Go loop (an N+1 that holds a job worker once the catalog balloons past the
-- `filler.fetch.max_catalog_clips` auto-fetch throttle) or dual-dialect `WITH RECURSIVE` — the exact
-- SQLite/Postgres divergence §7.2's search decision refused.
--
--   taxa_closure  one row per (ancestor, descendant) pair in the graph's transitive closure,
--                 INCLUDING the self-pair (ancestor = descendant), so a single join both writes the
--                 asserted leaf and expands its rollups. ~55 taxa ⇒ a few hundred rows. It is a
--                 DERIVED cache of `taxa`, rebuilt from the Go Forest (taxonomy.Forest.Ancestors — the
--                 single owner of the walk) whenever the GRAPH edits, which is rare. The clip rollup
--                 rebuild then joins clip leaves against this table:
--                   INSERT INTO clip_tags (clip_hash, taxon, leaf)
--                   SELECT l.clip_hash, c.ancestor, (c.ancestor = c.descendant)
--                   FROM (asserted leaves) l JOIN taxa_closure c ON c.descendant = l.taxon
--                 — identical SQL on both dialects, no recursion on the read/rebuild path.
--
-- ⚠ Like clip_tags, this is a synced cache: rebuildable from `taxa`, so forward-only (§16) and it
-- protects no data a backup must keep beyond what regenerates.
CREATE TABLE taxa_closure (
  ancestor   TEXT NOT NULL,
  descendant TEXT NOT NULL,
  PRIMARY KEY (descendant, ancestor)   -- keyed descendant-first: the rollup join filters on descendant
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
