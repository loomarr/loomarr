-- +goose Up
-- V37: filler sources become ONE FLAT LIST — every source is a row, whatever backs it.
--
-- This SUPERSEDES the derived/registered split that 00023 and 00026 established. Those
-- migrations' reasoning was sound and is recorded there; §10 ("Sources are one flat list")
-- records why it is being replaced and, more importantly, which of its properties survive.
--
-- ⚠ **The superseded rule was load-bearing.** 00023 said a row and a setting must never describe
-- the same source, "and that asymmetry is why one source never appears twice". Flattening does
-- not make that concern go away — it moves it here, where it becomes an INVARIANT THIS FILE
-- ENFORCES rather than a shape the old model made impossible by construction.
--
-- Two properties the flat list must carry itself:
--
-- 1. **"Not configured" stays expressible.** The derived rows could say "you could set up a
--    drop-folder but have not" — §10's own answer to "why is my catalog empty?". A table of
--    things-that-exist cannot say that, so `folder` and `library` are materialised as rows even
--    when their setting is unset, with a blank `uri` meaning exactly "not configured".
-- 2. **No source appears twice.** `folder` and `library` are SINGLETONS: exactly one row each,
--    created here, never inserted by an operator and never removable. The unique index below is
--    what makes a second one impossible — in the database, not by a Go guard that the next
--    caller forgets. Without it the flat model's failure mode is two drop-folder rows with
--    different switches, which is the precedence question 00023 refused to have.
--
-- `kind` gains `folder` / `library` / `youtube` alongside the existing `archive`. ⚠ `packs` is
-- deliberately NOT among them: there is no pack index to read (no URL, no manifest, no fetcher),
-- and §10 forbids a row that dims and changes nothing.

-- The singleton guard. A partial index so it binds ONLY the config-backed kinds: an operator may
-- add as many archive collections and YouTube playlists as they like.
CREATE UNIQUE INDEX IF NOT EXISTS idx_filler_sources_singleton
  ON filler_sources(kind) WHERE kind IN ('folder', 'library');

-- The two config-backed rows, materialised so the list is complete on every install.
--
-- ⚠ `uri` is left EMPTY on purpose. It is not a default path — it means "not configured", and the
-- API fills the live value from `filler.dir` at read time so that changing the setting does not
-- require a write here. Seeding it with a guessed path would make an unconfigured install claim
-- a folder it does not have.
--
-- ⚠ `enabled` is 1 for both. For `folder` this column is NOT the source of truth — the setting
-- `filler.source.folder.enabled` remains that, and the API projects it — so a 0 here would be a
-- second opinion about the same fact. It is 1 so that the projection has a neutral starting
-- point, never a contradicting one. For `library` it means "these clips count", not "a scan is
-- running": nothing scans a library, which is why that row stays switchless (§10).
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'folder', 'folder', '', 'Drop folder', '', 0, unixepoch(), 1
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE kind = 'folder');

INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'library', 'library', '', 'Media server library', '', 0, unixepoch(), 1
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE kind = 'library');

-- Forward-only (§16). Existing archive rows are untouched: their kind was already 'archive',
-- which the flat vocabulary keeps.

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
