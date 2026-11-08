-- +goose Up
-- The maintainer's label on a clip's defect record (#2034 step 4c): pass or fail against a good
-- copy of its era, and the era it was labelled against. The household report scores the floor and
-- its judge against these. A label describes the bytes, so it lives on the record and goes with it
-- when a re-key or merge drops the record.
ALTER TABLE filler_clip_defects ADD COLUMN label TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_clip_defects ADD COLUMN label_era INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_clip_defects ADD COLUMN labelled_at INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
