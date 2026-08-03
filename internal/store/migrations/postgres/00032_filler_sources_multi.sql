-- +goose Up
-- V38c: many folders and libraries, plus per-source fetch overrides (§10). Postgres mirror of the
-- sqlite 00032; the full reasoning lives there and in §10 ("Many folders, many libraries").
--
-- ═══ 1. The singleton index is DROPPED ═══
--
-- 00029 allowed exactly one folder and one library row. The concern was right — one folder must
-- not appear as two rows — but the instrument forbade something ordinary (commercials in two
-- places) without preventing the actual duplication. The invariant moves to the TARGET:
CREATE UNIQUE INDEX IF NOT EXISTS idx_filler_sources_uri ON filler_sources(kind, uri)
  WHERE uri <> '';

-- ⚠ `WHERE uri <> ''` matters: a seeded-but-unconfigured row has a BLANK uri (that is how "not
-- configured" is expressed, §10), and a plain unique index would allow only one blank row across
-- the table. A fresh install has two — folder and library.
DROP INDEX IF EXISTS idx_filler_sources_singleton;

-- ═══ 2. Per-source fetch overrides ═══
--
-- ⚠ **NULL = inherit the global; 0 = never auto-fetch this source.** They cannot share an
-- encoding, because `filler.fetch.every = 0` already means "off" — a NOT NULL DEFAULT 0 column
-- would silently read as "every existing source is switched off" on upgrade. That is the 00026
-- mistake: a default chosen for new rows, applied to old ones.
--
-- ⚠ Plain INTEGER, nullable, in BOTH dialects — no BOOLEAN here, so this migration does not
-- repeat the type split that bit 00029/00030/00031. Worth stating because the reflex after three
-- such traps is to assume every column differs; these do not.
ALTER TABLE filler_sources ADD COLUMN fetch_every_seconds INTEGER;
ALTER TABLE filler_sources ADD COLUMN fetch_max_per_run INTEGER;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
