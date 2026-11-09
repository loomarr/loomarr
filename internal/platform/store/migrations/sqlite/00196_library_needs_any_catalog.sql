-- +goose Up
-- #2151 provider-neutral Library: a Needs you answer and a search result name their title by any
-- catalog's id, namespace and value, not a TMDB id. Every answer and result so far is TMDB's.
ALTER TABLE library_answers RENAME COLUMN tmdb_id TO external_id;
ALTER TABLE library_answers ADD COLUMN namespace TEXT NOT NULL DEFAULT '';
UPDATE library_answers SET namespace = 'tmdb' WHERE external_id <> '';

ALTER TABLE library_candidates RENAME COLUMN tmdb_id TO external_id;
ALTER TABLE library_candidates ADD COLUMN namespace TEXT NOT NULL DEFAULT '';
UPDATE library_candidates SET namespace = 'tmdb';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
