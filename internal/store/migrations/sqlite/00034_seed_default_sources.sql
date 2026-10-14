-- +goose Up
-- V38c.8: a fresh install ships with fetchable sources (§10, maintainer 2026-08-03).
--
-- 00029 seeded only `folder` and `library`. Both are SCANNED, so a new install had nothing it
-- could fetch and an operator had to know what to add before filler could fill anything.
--
-- ⚠ **Seeding a source downloads NOTHING.** A row records that a source exists and is allowed;
-- fetching goes through the pull's approval gate or a deliberate per-result queue. That is what
-- makes it safe to add these on the operator's behalf — the same promise the Add-a-source copy
-- makes on screen.
--
-- ⚠ **Each identifier was VERIFIED against the live archive.org API** (2026-08-03), not guessed.
-- Five plausible-looking ones returned zero items (`classic_tv_ads`, `televisionads`,
-- `vintage_tv_commercials`, `tvcommercials`, `commercialsandbumpers`), which is exactly why the
-- fixture prose in the mock ("11 curated collections") is not a list to copy. The large
-- collections that DO exist — mirrortube (1.2M mirrored YouTube videos), television (735K),
-- vhsvault (116K) — are general video, and pointing auto-fetch at them would pull arbitrary
-- long-form content into a catalog meant for 30-second breaks.
--
-- ⚠ **`license` is deliberately EMPTY on all three.** All three declare none, and §10 defines
-- empty as UNKNOWN — never "public domain". ~92% of archive items declare nothing, so absence is
-- the norm and carries no permission. Writing a reassuring default here would have Loomarr
-- asserting a legal fact nobody checked.
--
-- ⚠ **`label` carries a HUMAN-READABLE name.** `vhscommercials` is not something an operator
-- recognises; the Sources row renders the label with the target beneath it.
--
-- ⚠ Guarded by id, not by kind. 00029's singleton guard was `WHERE NOT EXISTS (… kind = 'folder')`
-- because there was exactly one of those; V38c allows MANY archive rows, so a kind guard would
-- skip the whole seed the moment an operator had added any collection of their own.
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'archive:classic_tv_commercials', 'archive', 'classic_tv_commercials',
       'Classic TV Commercials', '', 0, unixepoch(), 1
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE id = 'archive:classic_tv_commercials');

INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'archive:vhscommercials', 'archive', 'vhscommercials',
       'Commercials From The Vault', '', 0, unixepoch(), 1
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE id = 'archive:vhscommercials');

INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'archive:tv_ads', 'archive', 'tv_ads', 'TV Ads', '', 0, unixepoch(), 1
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE id = 'archive:tv_ads');

-- ⚠ The YouTube row seeds with an EMPTY uri, and that is the point rather than an omission. §10
-- says Loomarr "never recommends YouTube content itself" — the operator brings their own
-- playlist — so a seeded target would be exactly the recommendation that rule forbids. The mock
-- draws the same row: present, reading "Bring your own playlist", stat "no playlist yet".
--
-- An empty uri also keeps it out of every pull plan on its own: `Fetchable()` requires a non-empty
-- uri, so this row cannot be handed to the ingest job until someone fills it in.
INSERT INTO filler_sources (id, kind, uri, label, license, last_fetched_at, created_at, enabled)
SELECT 'youtube', 'youtube', '', 'YouTube', '', 0, unixepoch(), 1
WHERE NOT EXISTS (SELECT 1 FROM filler_sources WHERE kind = 'youtube');

-- +goose Down
-- No down: forward-only (§16).
