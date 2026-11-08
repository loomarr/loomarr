-- +goose Up
-- Filmographies (#2151 decision 50): the movies and series of the people who directed or created a
-- title, are among a title's ten top-billed actors, or are credited on three or more titles, as
-- TMDB lists them. A title the library owns links through library_ids (namespace tmdb) and
-- library_items.kind. Fetched when a person first appears and monthly after; deleted with the person.
ALTER TABLE library_people ADD COLUMN filmography_at_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE library_people ADD COLUMN filmography_retry_at_ms INTEGER NOT NULL DEFAULT 0;

CREATE TABLE library_filmography (
    person_id TEXT NOT NULL REFERENCES library_people(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    kind TEXT NOT NULL,
    tmdb_id TEXT NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    role TEXT NOT NULL,
    character_name TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (person_id, position)
);
CREATE INDEX idx_library_filmography_title ON library_filmography (kind, tmdb_id);

-- +goose Down
-- No down: forward-only (§16).
