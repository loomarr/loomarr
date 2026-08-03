-- +goose Up
-- V38c: many folders and libraries, plus per-source fetch overrides (§10).
--
-- Two changes, both reversing or extending something V37 established one phase earlier.
--
-- ═══ 1. The singleton index is DROPPED ═══
--
-- 00029 added a partial unique index on `kind IN ('folder','library')` so there could be exactly
-- one of each. ⚠ **The concern behind it was right and the instrument was wrong.** What must not
-- happen is ONE folder appearing as TWO rows — a stale row disagreeing with the setting about the
-- same directory. Forbidding a second DISTINCT folder does not prevent that, and it forbids
-- something ordinary: commercials living in two places, which V37 gave no expression at all.
--
-- The invariant moves to where it actually belongs — the TARGET:
CREATE UNIQUE INDEX IF NOT EXISTS idx_filler_sources_uri ON filler_sources(kind, uri)
  WHERE uri <> '';

-- ⚠ `WHERE uri <> ''` matters. A seeded-but-unconfigured folder row has a BLANK uri (that is how
-- "you could set up a drop-folder but have not" is expressed — §10 property 1), and a plain
-- unique index would allow only one blank row across the whole table. A fresh install has two:
-- the folder and the library.
DROP INDEX IF EXISTS idx_filler_sources_singleton;

-- ═══ 2. Per-source fetch overrides ═══
--
-- ⚠ **NULL means INHERIT the global; 0 means NEVER auto-fetch this source.** They cannot share an
-- encoding, because `filler.fetch.every = 0` already means "off" — so a NOT NULL DEFAULT 0 column
-- would read as "every existing source is switched off" on upgrade, silently.
--
-- That is precisely the 00026 mistake this file is written to avoid: a default chosen for NEW
-- rows, applied to old ones. Nullable is not a style preference here; it is the only encoding
-- that can express three states (inherit / never / every N) in one column.
ALTER TABLE filler_sources ADD COLUMN fetch_every_seconds INTEGER;
ALTER TABLE filler_sources ADD COLUMN fetch_max_per_run INTEGER;

-- ⚠ The catalog and disk ceilings stay GLOBAL and get no column. They bound the whole install —
-- the operator is protecting one disk, not one source — and per-source disk caps would let four
-- sources each stay under their limit while together filling the volume.
--
-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
