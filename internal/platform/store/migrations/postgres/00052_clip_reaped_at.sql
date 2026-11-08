-- +goose Up
-- V54 (§10). Postgres mirror of the sqlite 00052; the full reasoning lives there.
--
-- In short: the split sweep deletes a spent compilation's RECORDING but must keep its ROW, or the
-- next `filler-sync` prunes the row for a missing file and dangles `parent_hash` on every clip cut
-- out of it. `reaped_at` marks the tombstone: sync skips it, lineage resolves, the UI can say
-- "recording reclaimed", and the split rung stops re-proposing a reel it can no longer read.
ALTER TABLE clips ADD COLUMN reaped_at BIGINT;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
