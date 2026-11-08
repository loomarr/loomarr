-- +goose Up
-- #2151 decision 56: a movie no arr or file names is matched by a TMDB search when exactly one
-- result agrees on its name, year and length. That match is kept as an answer like the
-- maintainer's, so it follows the title's scan origin, and `answered_by` says who gave it: `you`
-- (Needs you, "Wrong title?") or `search` (the Matcher). Every answer so far is the maintainer's.

ALTER TABLE library_answers ADD COLUMN answered_by TEXT NOT NULL DEFAULT 'you';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
