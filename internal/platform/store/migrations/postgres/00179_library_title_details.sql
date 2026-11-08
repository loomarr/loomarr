-- +goose Up
-- Full title details (#2151 decision 46). A title keeps its tagline and the web page of its
-- trailer, and its production countries are rows of their own, in the source's order; an episode
-- has none (they are its show's, decision 42). Rewritten with each inventory write (decision 43)
-- and filled by the write-version bump (decision 55).
ALTER TABLE library_items ADD COLUMN tagline TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN trailer TEXT NOT NULL DEFAULT '';

CREATE TABLE library_countries (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE INDEX idx_library_countries_name ON library_countries (name);

-- +goose Down
-- No down: forward-only (§16).
