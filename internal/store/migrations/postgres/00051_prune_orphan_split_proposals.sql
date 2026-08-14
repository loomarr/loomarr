-- +goose Up
-- V54 (§10). Postgres mirror of the sqlite 00051; the full reasoning lives there.
--
-- In short: `filler_split_proposals` is a no-foreign-key sibling of `clips`, so nothing pruned it.
-- A catalog wipe left 48 proposals behind, which Incoming rendered as hash-titled "compilations to
-- review" pointing at deleted files. The code fix prunes inside `DeleteClipsNotIn`; this heals
-- installs whose orphans already exist.
--
-- ⚠ Narrow on purpose: a proposal whose compilation is still catalogued is live review work.
DELETE FROM filler_split_proposals
WHERE NOT EXISTS (SELECT 1 FROM clips c WHERE c.hash = filler_split_proposals.clip_hash);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
