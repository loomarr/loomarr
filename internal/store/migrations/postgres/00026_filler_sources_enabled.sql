-- +goose Up
-- V35: a filler source can be switched off.
--
-- The Sources tab gives every source an on/off switch whose stated meaning is a behaviour
-- claim, not decoration: "Loomarr stops scanning, searching and downloading from it. Clips
-- already in the catalog stay put."
--
-- ⚠ **Disabling is NOT a delete, and the column is what keeps those separate.** A source the
-- operator switched off still has its rows, its licence and its fetch history, so switching it
-- back on restores exactly what was there rather than starting over. Deleting the row is the
-- other action, and the UI offers both.
--
-- ⚠ **Only REMOTE sources get a column.** The drop-folder and the media-server library are
-- DERIVED from configuration (00023 records why: a row and a setting must never describe the
-- same source), so their switches are settings — `filler.source.folder.enabled` and
-- `filler.source.library.enabled`. Adding a row here for them would recreate exactly the
-- precedence question 00023 exists to avoid.
--
-- Default 1: every source that exists today was added deliberately and is in use. A default of
-- 0 would silently stop fetching for every install on upgrade.

ALTER TABLE filler_sources ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
