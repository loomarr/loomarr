-- +goose Up
-- Library → Items (#2151 step 10): what the scanner wrote, in a shape the Library pages can list
-- and search without decoding every inventory observation.
--
-- `library_items` is one row per movie and series the scanner wrote: its inventory item id, kind
-- and the resolved name, sort name and year. Whether it is present, and whether its files' headers
-- are read, come from its files: `library_files.item_id` is the item a file was written as, and
-- `series_id` is an episode's series ('' for none).
CREATE TABLE library_items (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    sort_name TEXT NOT NULL,
    year INTEGER NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL
);

CREATE INDEX library_items_sort ON library_items (kind, sort_name);

ALTER TABLE library_files ADD COLUMN item_id TEXT NOT NULL DEFAULT '';
ALTER TABLE library_files ADD COLUMN series_id TEXT NOT NULL DEFAULT '';
CREATE INDEX library_files_item ON library_files (item_id);
CREATE INDEX library_files_series ON library_files (series_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
