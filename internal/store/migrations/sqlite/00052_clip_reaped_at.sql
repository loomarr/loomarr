-- +goose Up
-- V54 (§10): a composite whose recording has been reclaimed keeps its ROW.
--
-- The split sweep deletes the recording of a compilation that has already yielded clips and whose
-- remaining cuts nobody reviewed. Deleting the FILE is the point — a reel is 1–2 GB and the disk is
-- finite — but deleting the ROW would take the lineage with it.
--
-- ⚠ **The cascade this column exists to stop.** Every clip cut out of a reel carries
-- `parent_hash = <the composite>`. Remove the file and the next `filler-sync` finds the clip gone
-- from its source and prunes the row (`DeleteClipsNotIn`) — dangling `parent_hash` on all 47
-- children at once, so V45's whole keep-the-parent lineage story collapses on the first sweep.
--
-- So the row survives as a TOMBSTONE: `reaped_at` set, file gone. `DeleteClipsNotIn` skips a
-- reaped row (its absence from the scan is expected, not news), `parent_hash` keeps resolving, and
-- the UI can say "recording reclaimed" rather than offering a play button that 404s.
--
-- ⚠ It also STOPS RE-PROPOSAL, which is the difference between a sweep and a churn loop. The
-- composite is still `is_composite`, so without this the split rung would re-detect it on the next
-- pass — propose → partly confirm → leftovers → sweep → re-propose, burning a boundary scan every
-- cycle and never converging. A reaped clip is not a candidate.
--
-- Nullable, no default: `NULL` means "the recording is still there", which is every existing row.
ALTER TABLE clips ADD COLUMN reaped_at INTEGER;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
