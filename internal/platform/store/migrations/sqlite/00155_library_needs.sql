-- +goose Up
-- Library → Needs you (#2151 step 11, decisions 3 and 27): a title nothing grounds gets a ranked
-- TMDB candidate, and the maintainer answers it.
--
-- `library_items` learns the title's scan origin (the file of a movie, the folder of a series),
-- whether anything grounds it, and what was last searched for it ('' never).
--
-- `library_answers` is the maintainer's answer for a scan origin: a TMDB id it is linked to (an
-- explicit link, decision 0027), or ignored ("not a title"). It outlives the title's rewrites.
--
-- `library_candidates` is a title's search result: its candidates in rank order, with whether the
-- name, year and length agree with the file.
ALTER TABLE library_items ADD COLUMN origin_authority TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN origin_key TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN grounded INTEGER NOT NULL DEFAULT 1;
ALTER TABLE library_items ADD COLUMN searched TEXT NOT NULL DEFAULT '';
CREATE INDEX library_items_origin ON library_items (origin_authority, origin_key);

CREATE TABLE library_answers (
    origin_authority TEXT NOT NULL,
    origin_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    tmdb_id TEXT NOT NULL DEFAULT '',
    ignored INTEGER NOT NULL DEFAULT 0,
    answered_at INTEGER NOT NULL,
    PRIMARY KEY (origin_authority, origin_key)
);

CREATE TABLE library_candidates (
    item_id TEXT NOT NULL,
    rank INTEGER NOT NULL,
    kind TEXT NOT NULL,
    tmdb_id TEXT NOT NULL,
    name TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    overview TEXT NOT NULL DEFAULT '',
    poster TEXT NOT NULL DEFAULT '',
    name_match INTEGER NOT NULL DEFAULT 0,
    year_match INTEGER NOT NULL DEFAULT 0,
    length_match INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (item_id, rank)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
