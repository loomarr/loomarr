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
-- ⚠ **Only REMOTE sources get a column.** The drop-folder is DERIVED from configuration (00023
-- records why: a row and a setting must never describe the same source), so its switch is the
-- `filler.source.folder.enabled` setting. Adding a row here for it would recreate exactly the
-- precedence question 00023 exists to avoid.
--
-- ⚠ The media-server library row gets NO switch at all — nothing scans a library for clips (§10
-- took the media server out of the filler path), so a control there would change nothing. An
-- earlier draft of this comment named a `filler.source.library.enabled` setting; that key was
-- dropped before it shipped, once implementing it found there was no scan to gate.
--
-- Default 1: every source that exists today was added deliberately and is in use. A default of
-- 0 would silently stop fetching for every install on upgrade.

ALTER TABLE filler_sources ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
