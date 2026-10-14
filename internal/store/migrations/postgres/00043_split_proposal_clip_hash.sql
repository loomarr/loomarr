-- +goose Up
-- V51a (§10). Postgres mirror of the sqlite 00043; the full reasoning lives there.
--
-- ⚠ In short: `Propose` wrote the clip's PATH into this column and `Confirm` looked the clip up
-- by it against a hash-keyed `GetClip`, so no split could ever be committed. The column is renamed
-- so the name cannot lie about the value again, and pending rows are dropped because they were
-- never confirmable in the first place.
DELETE FROM filler_split_proposals;

ALTER TABLE filler_split_proposals RENAME COLUMN clip_path TO clip_hash;

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
