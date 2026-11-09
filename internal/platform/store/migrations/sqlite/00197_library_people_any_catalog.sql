-- +goose Up
-- #2151 provider-neutral Library: a person and a filmography title name any catalog's id,
-- namespace and value, not a TMDB id. Every linked person and filmography title so far is TMDB's;
-- a person's row id ('tmdb:<id>') already says so.
ALTER TABLE library_people RENAME COLUMN tmdb_id TO external_id;
ALTER TABLE library_people ADD COLUMN namespace TEXT NOT NULL DEFAULT '';
UPDATE library_people SET namespace = 'tmdb' WHERE external_id <> '';

DROP INDEX idx_library_filmography_title;
ALTER TABLE library_filmography RENAME COLUMN tmdb_id TO external_id;
ALTER TABLE library_filmography ADD COLUMN namespace TEXT NOT NULL DEFAULT '';
UPDATE library_filmography SET namespace = 'tmdb';
CREATE INDEX idx_library_filmography_title ON library_filmography (kind, namespace, external_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
