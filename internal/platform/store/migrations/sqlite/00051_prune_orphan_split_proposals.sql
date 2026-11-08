-- +goose Up
-- V54 (§10): a split proposal whose compilation is no longer in the catalog is unactionable.
--
-- ⚠ `filler_split_proposals` is a sibling of `clips` with NO foreign key — deliberately, the same
-- independence `filler_clip_pipeline` has, so it survives a `clips` rebuild. The price of that
-- independence is that nothing cleaned it up. Measured 2026-08-11: deleting every clip file and
-- running filler-sync pruned `clips` to 0 and left **48** proposals behind, which Incoming rendered
-- as 48 "compilations to review" titled with raw 64-character hashes — the display name falls back
-- to the clip identity when `GetClip` misses — each offering a Review-cuts button that opens a
-- review of a file that is gone.
--
-- The code fix is a prune inside `DeleteClipsNotIn` (the sync's prune, beside the pipeline one it
-- copies). That handles every future wipe, so this statement exists only to heal installs whose
-- orphans already exist rather than making the operator wait for a sync they did not ask for.
--
-- ⚠ **NARROW, unlike 00043's `DELETE FROM filler_split_proposals`.** That one was right because no
-- pending proposal was confirmable at all. Here a proposal whose compilation is still catalogued is
-- live review work an operator may be halfway through editing, and deleting it to tidy up a
-- different problem would destroy exactly what the review queue is for.
DELETE FROM filler_split_proposals
WHERE NOT EXISTS (SELECT 1 FROM clips c WHERE c.hash = filler_split_proposals.clip_hash);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
