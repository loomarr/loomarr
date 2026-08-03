-- +goose Up
-- V37: filler sources become ONE FLAT LIST — every source is a row, whatever backs it.
--
-- Postgres mirror of the sqlite 00029. The full reasoning lives there and in §10 ("Sources are
-- one flat list"); the short version is that this SUPERSEDES the derived/registered split from
-- 00023/00026, and the two properties that split protected have to be carried by the flat model
-- instead of falling out of its shape:
--
-- 1. **"Not configured" stays expressible** — `folder`/`library` exist as rows even when their
--    setting is unset, with a blank `uri` meaning exactly that.
-- 2. **No source appears twice** — those two kinds are SINGLETONS, enforced by the partial unique
--    index below rather than by a Go guard the next caller forgets. Without it the flat model's
--    failure mode is two drop-folder rows with different switches, which is the precedence
--    question 00023 refused to have.
--
-- ⚠ Both dialects use a PARTIAL unique index (`WHERE kind IN (...)`), which Postgres and modern
-- SQLite both support. It binds only the config-backed kinds: an operator may add as many
-- archive collections and YouTube playlists as they like.

CREATE UNIQUE INDEX IF NOT EXISTS idx_filler_sources_singleton
  ON filler_sources(kind) WHERE kind IN ('folder', 'library');

-- The two config-backed rows. `uri` is EMPTY meaning "not configured" — never a guessed path —
-- and the API fills the live value from `filler.dir` at read time. `enabled` starts TRUE as a
-- neutral value: for `folder` the setting `filler.source.folder.enabled` remains the source of
-- truth and the API projects it, so FALSE here would be a second opinion about the same fact.
--
-- ⚠ TWO dialect differences in one statement, both easy to miss by copying the sqlite file:
-- `EXTRACT(EPOCH FROM NOW())::BIGINT` rather than sqlite's `unixepoch()`, and `enabled` is
-- BOOLEAN here (`TRUE`) where sqlite has INTEGER (`1`). A literal `1` is a hard 42804 at migrate
-- time — the good failure, since it cannot reach an install — and `test-pg` is what caught it.
-- Reading the two files side by side did not.
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'folder', 'folder', '', 'Drop folder', '', 0, EXTRACT(EPOCH FROM NOW())::BIGINT, TRUE
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE kind = 'folder');

INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'library', 'library', '', 'Media server library', '', 0, EXTRACT(EPOCH FROM NOW())::BIGINT, TRUE
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE kind = 'library');

-- Forward-only (§16). Existing archive rows are untouched: their kind was already 'archive'.

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
