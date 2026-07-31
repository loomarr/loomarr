-- +goose Up
-- §12 guide hover card: a clip's resolution, so a viewer looking at a break can see that the
-- grainy advert is a 240p capture rather than a playback fault.
--
-- ⚠ **AMENDED BY V17c (2026-07-31).** This used to say quality NEVER affects pod selection. It
-- is now display-only BY DEFAULT — `filler.min_quality` is an opt-in floor, unset by default,
-- and with it unset selection is byte-identical to before. Full rationale on the SQLite twin.
--
-- Derived from the video stream's height at scan time (filler.QualityFromHeight): "1080p",
-- "480p", "" when the file has no video stream. Not backfilled: clips scanned before this
-- column existed have no quality until the next sync re-probes them, and an empty string
-- renders as no badge — the honest state, not "unknown".
--
-- Safe as a plain ADD COLUMN: `clips` is a synced CACHE (see 00013), so the worst case is one
-- sync cycle with blank badges.
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN quality TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
