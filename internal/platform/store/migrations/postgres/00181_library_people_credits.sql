-- +goose Up
-- People and credits (#2151 decisions 45, 49). A person with a TMDB id is one row wherever they
-- are credited (id 'tmdb:<id>'); one a source names without it is kept by name, unlinked
-- (id 'name:<name>'). A title's credits are its cast in billing order, then its key crew; an
-- episode has none of its own (decision 42). Credits are rewritten with each inventory write
-- (decision 43) and filled by the write-version bump (decision 55); a person outlives the titles
-- that credited them.
CREATE TABLE library_people (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    tmdb_id TEXT NOT NULL DEFAULT '',
    photo TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_library_people_name ON library_people (name);

CREATE TABLE library_credits (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    person_id TEXT NOT NULL REFERENCES library_people(id),
    role TEXT NOT NULL,
    character_name TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, position)
);
CREATE INDEX idx_library_credits_person ON library_credits (person_id, role);

-- +goose Down
-- No down: forward-only (§16).
