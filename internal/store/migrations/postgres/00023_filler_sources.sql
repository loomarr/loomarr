-- +goose Up
-- V33: the persisted filler-sources registry + a clip's declared licence. Postgres twin of the
-- SQLite migration — see that file for why this reverses V28's read-model decision, why the TABLE
-- WINS with `filler.dir` seeding it, and why `seeded_from` exists (it stops a changed setting
-- going silently inert).
--
-- ⚠ `license` empty means UNKNOWN, never "public domain": ~92% of archive.org items declare none.

CREATE TABLE IF NOT EXISTS filler_sources (
  id              TEXT PRIMARY KEY,
  kind            TEXT NOT NULL,
  uri             TEXT NOT NULL,
  label           TEXT NOT NULL DEFAULT '',
  license         TEXT NOT NULL DEFAULT '',
  seeded_from     TEXT NOT NULL DEFAULT '',
  last_fetched_at BIGINT NOT NULL DEFAULT 0,
  created_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_filler_sources_kind ON filler_sources(kind, created_at);

ALTER TABLE clips ADD COLUMN license TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
