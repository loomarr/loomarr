-- +goose Up
-- §12 guide hover card: a clip's resolution, so a viewer looking at a break can see that the
-- grainy advert is a 240p capture rather than a playback fault. Display-only — quality NEVER
-- affects pod selection, or a well-meaning "prefer HD" would quietly starve the era-accurate
-- 4:3 commercials the whole feature exists to play.
--
-- Derived from the video stream's height at scan time (filler.QualityFromHeight): "1080p",
-- "480p", "" when the file has no video stream. Nullable-by-default rather than backfilled:
-- clips scanned before this column existed simply have no quality until the next sync
-- re-probes them, and an empty string renders as no badge — the honest state, not "unknown".
--
-- Safe as a plain ADD COLUMN: `clips` is a synced CACHE (see 00013), so the worst case is one
-- sync cycle with blank badges.
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN quality TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
