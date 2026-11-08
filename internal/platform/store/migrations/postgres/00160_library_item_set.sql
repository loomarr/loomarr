-- +goose Up
-- Library → Collections, ready-made sets (#2151 step 12, decision 28): the TMDB collection a movie
-- belongs to, as the library_catalog key of the set's own record ('' none). The set's record says
-- its name and every movie in it, so the shelf can say "you have 3 of 4".
ALTER TABLE library_items ADD COLUMN set_key TEXT NOT NULL DEFAULT '';
CREATE INDEX library_items_set ON library_items (set_key);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
