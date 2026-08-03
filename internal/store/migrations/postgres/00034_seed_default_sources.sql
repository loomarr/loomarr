-- +goose Up
-- V38c.8: a fresh install ships with fetchable sources (§10, maintainer 2026-08-03).
--
-- The sqlite twin carries the full reasoning; the short version is that 00029 seeded only the two
-- SCANNED kinds, so a new install had nothing it could fetch. Seeding a source downloads NOTHING —
-- a row records that a source exists and is allowed, and fetching still goes through the approval
-- gate.
--
-- ⚠ Identifiers VERIFIED against the live archive.org API (2026-08-03), not guessed — five
-- plausible-looking ones returned zero items. ⚠ `license` is empty because all three declare
-- none, and §10 defines empty as UNKNOWN, never "public domain". ⚠ `label` is the human-readable
-- name; the row renders it with the target beneath.
--
-- ⚠ Guarded by id rather than kind: V38c allows many archive rows, so a kind guard would skip the
-- seed entirely once an operator had added a collection of their own.
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'archive:classic_tv_commercials', 'archive', 'classic_tv_commercials',
       'Classic TV Commercials', '', 0, EXTRACT(EPOCH FROM NOW())::BIGINT, TRUE
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE id = 'archive:classic_tv_commercials');

INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'archive:vhscommercials', 'archive', 'vhscommercials',
       'Commercials From The Vault', '', 0, EXTRACT(EPOCH FROM NOW())::BIGINT, TRUE
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE id = 'archive:vhscommercials');

INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'archive:tv_ads', 'archive', 'tv_ads', 'TV Ads', '',
       0, EXTRACT(EPOCH FROM NOW())::BIGINT, TRUE
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE id = 'archive:tv_ads');

-- ⚠ EMPTY uri, deliberately: §10 says Loomarr never recommends YouTube content itself, so the
-- operator brings the playlist. An empty uri also fails `Fetchable()`, keeping the row out of
-- every pull plan until someone fills it in.
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'youtube', 'youtube', '', 'YouTube', '', 0, EXTRACT(EPOCH FROM NOW())::BIGINT, TRUE
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE kind = 'youtube');

-- +goose Down
-- No down: forward-only (§16).
