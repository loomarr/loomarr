-- +goose Up
-- Download progress on titles (§4, §18.1 — Phase 4 arr queue poller). When the direct
-- Sonarr/Radarr requester is used, the arr-queue-poll job persists each downloading title's
-- progress here so GET /v1/titles reads it straight from the store (survives a restart, no
-- separate cache). All nullable/zero-defaulted: a title that never downloads (Seerr path, or
-- already available) simply carries 0/"".
--   progress        — 0..1 completion fraction (1 - sizeleft/size)
--   eta_text        — the arr's human time-left string, passed through for display
--   download_status — the arr's queue status ("downloading"/"warning"/"stalled"/…), so a
--                     stuck download reads as such rather than fake healthy progress
--
-- Forward-only (§16). ALTER in place — titles holds durable rows.

ALTER TABLE titles ADD COLUMN progress REAL NOT NULL DEFAULT 0;
ALTER TABLE titles ADD COLUMN eta_text TEXT NOT NULL DEFAULT '';
ALTER TABLE titles ADD COLUMN download_status TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE titles DROP COLUMN progress;
ALTER TABLE titles DROP COLUMN eta_text;
ALTER TABLE titles DROP COLUMN download_status;
