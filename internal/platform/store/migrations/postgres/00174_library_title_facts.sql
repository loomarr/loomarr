-- +goose Up
-- Rich library data, titles (#2151 decisions 40, 42, 45; ADR 0047). library_items holds episodes
-- too, and one resolved answer per fact in a column, with the provider that supplied it in its
-- `_from` column ('' unknown; an episode's rating_from is 'show' when the rating is its show's).
-- An episode names its series by the series' scan origin (series_authority, series_key); the store
-- fills series_id once both rows exist. Ids, genres, studios and tags are rows of their own, in
-- the order the source gave them. All are rewritten with each inventory write (decision 43) and
-- filled by the write-version bump (decision 55).
ALTER TABLE library_items ADD COLUMN original_title TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN overview TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN premiere_date TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN rating TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN score DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE library_items ADD COLUMN runtime_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE library_items ADD COLUMN season INTEGER NOT NULL DEFAULT 0;
ALTER TABLE library_items ADD COLUMN episode INTEGER NOT NULL DEFAULT 0;
ALTER TABLE library_items ADD COLUMN episode_end INTEGER NOT NULL DEFAULT 0;
ALTER TABLE library_items ADD COLUMN series_authority TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN series_key TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN series_id TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN name_from TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN year_from TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN overview_from TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN premiere_from TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN rating_from TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN score_from TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN runtime_from TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_library_items_series_origin ON library_items (series_authority, series_key);
CREATE INDEX idx_library_items_series ON library_items (series_id);

CREATE TABLE library_ids (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (item_id, namespace, value)
);
CREATE INDEX idx_library_ids_value ON library_ids (namespace, value);

CREATE TABLE library_genres (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE INDEX idx_library_genres_name ON library_genres (name);

CREATE TABLE library_studios (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE INDEX idx_library_studios_name ON library_studios (name);

CREATE TABLE library_tags (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE INDEX idx_library_tags_name ON library_tags (name);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
