-- +goose Up
-- V35: "Remove from catalog" — a tombstone, not a delete.
--
-- The Catalog tab's bulk bar has a destructive "Remove from catalog" action. The obvious
-- implementation — DELETE the row — does not work, because `clips` is a synced CACHE of
-- FILLER_DIR (00013): the next scan finds the file still sitting there and puts the row back.
-- An operator would remove a clip, watch it reappear fifteen minutes later, and reasonably
-- conclude the button is broken.
--
-- ⚠ **The other obvious implementation — delete the FILE — is deliberately not what this is.**
-- Nothing in Loomarr deletes an operator's media. Disabling a source keeps its clips; deleting a
-- source keeps its clips ("they are real files, already tagged and possibly pinned into a
-- channel"). The button says *remove from the CATALOG*, and that is exactly what it does: the
-- file stays where the operator put it, and Loomarr stops using it.
--
-- So: a tombstone the scan preserves like it preserves tags. A removed clip is invisible to the
-- catalog listing and to pod assembly, and stays removed across re-scans — which is the property
-- a DELETE cannot give.
--
-- An epoch rather than a boolean, matching last_fetched_at and decided_at: it records WHEN, which
-- a restore path and any future history need, and 0 reads as "not removed" with no second column.

ALTER TABLE clips ADD COLUMN removed_at BIGINT NOT NULL DEFAULT 0;

-- The catalog listing and pod assembly both filter on this, and both read the whole table.
CREATE INDEX IF NOT EXISTS idx_clips_removed_at ON clips(removed_at);

-- Safe as a plain ADD COLUMN: `clips` is a synced cache (00013), so the worst case is one sync
-- cycle before the column means anything.
--
-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
