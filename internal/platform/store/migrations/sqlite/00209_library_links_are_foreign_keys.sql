-- +goose Up
-- Schema to a 10 (#2793, item 1): the library's links between its own rows are foreign keys.
--
--   - library_items.series_id, an episode's show, references library_items;
--   - library_files.inventory_item_id and inventory_series_id, the title and show a file was last
--     written as, reference library_items;
--   - library_candidates.inventory_item_id references library_items and goes with its title.
--
-- The three links used '' for "not known yet", which no row has as its id, so they become nullable
-- and the copy turns each '' into NULL. A link naming no row is cleared the same way: the next scan
-- writes it again. A candidate whose title is gone is dropped. Nothing deletes a title a file or an
-- episode still names (the catalog purge checks), so a restricting key changes no behaviour.
--
-- SQLite cannot add a foreign key in place, so library_items is rebuilt with its descendants (the
-- 00192 pattern): children are renamed first, created after the parent, and dropped first. Copies
-- keep their columns in order, so INSERT ... SELECT * carries every value across, except where a
-- link turns '' into NULL.

-- library_items and its descendants ---------------------------------------------------------------
ALTER TABLE library_streams RENAME TO library_streams_old;
ALTER TABLE library_chapters RENAME TO library_chapters_old;
ALTER TABLE library_markers RENAME TO library_markers_old;
ALTER TABLE library_trickplay RENAME TO library_trickplay_old;
ALTER TABLE library_sources RENAME TO library_sources_old;
ALTER TABLE library_ids RENAME TO library_ids_old;
ALTER TABLE library_genres RENAME TO library_genres_old;
ALTER TABLE library_studios RENAME TO library_studios_old;
ALTER TABLE library_countries RENAME TO library_countries_old;
ALTER TABLE library_credits RENAME TO library_credits_old;
ALTER TABLE library_tags RENAME TO library_tags_old;
ALTER TABLE library_art RENAME TO library_art_old;
ALTER TABLE library_item_images RENAME TO library_item_images_old;
ALTER TABLE library_candidate_images RENAME TO library_candidate_images_old;
ALTER TABLE library_collection_members RENAME TO library_collection_members_old;
ALTER TABLE library_items RENAME TO library_items_old;

CREATE TABLE library_items (
    id               TEXT    PRIMARY KEY,
    kind             TEXT    NOT NULL CHECK (kind IN ('movie', 'series', 'episode')),
    name             TEXT    NOT NULL,
    sort_name        TEXT    NOT NULL,
    year             INTEGER NOT NULL DEFAULT 0,
    updated_at_ms    INTEGER NOT NULL,
    catalog          TEXT    NOT NULL DEFAULT '',
    origin_authority TEXT    NOT NULL DEFAULT '',
    origin_key       TEXT    NOT NULL DEFAULT '',
    grounded         BOOLEAN NOT NULL DEFAULT TRUE CHECK (grounded IN (0, 1)),
    searched         TEXT    NOT NULL DEFAULT '',
    match_basis      TEXT    NOT NULL DEFAULT '',
    match_app        TEXT    NOT NULL DEFAULT '',
    match_page       TEXT    NOT NULL DEFAULT '',
    needs_noticed    BOOLEAN NOT NULL DEFAULT FALSE CHECK (needs_noticed IN (0, 1)),
    set_key          TEXT    NOT NULL DEFAULT '',
    original_title   TEXT    NOT NULL DEFAULT '',
    overview         TEXT    NOT NULL DEFAULT '',
    premiere_date    TEXT    NOT NULL DEFAULT '',
    rating           TEXT    NOT NULL DEFAULT '',
    score            REAL    NOT NULL DEFAULT 0,
    runtime_ms       INTEGER NOT NULL DEFAULT 0,
    season           INTEGER NOT NULL DEFAULT 0,
    episode          INTEGER NOT NULL DEFAULT 0,
    episode_end      INTEGER NOT NULL DEFAULT 0,
    series_authority TEXT    NOT NULL DEFAULT '',
    series_key       TEXT    NOT NULL DEFAULT '',
    series_id        TEXT    REFERENCES library_items(id),
    name_from        TEXT    NOT NULL DEFAULT '',
    year_from        TEXT    NOT NULL DEFAULT '',
    overview_from    TEXT    NOT NULL DEFAULT '',
    premiere_from    TEXT    NOT NULL DEFAULT '',
    rating_from      TEXT    NOT NULL DEFAULT '',
    score_from       TEXT    NOT NULL DEFAULT '',
    runtime_from     TEXT    NOT NULL DEFAULT '',
    tagline          TEXT    NOT NULL DEFAULT '',
    trailer          TEXT    NOT NULL DEFAULT '',
    unnumbered       BOOLEAN NOT NULL DEFAULT FALSE CHECK (unnumbered IN (0, 1))
);
CREATE TABLE library_ids (
    item_id   TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (item_id, namespace, value)
);
CREATE TABLE library_genres (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_studios (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_countries (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_credits (
    item_id        TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    position       INTEGER NOT NULL,
    person_id      TEXT    NOT NULL REFERENCES library_people(id),
    role           TEXT    NOT NULL,
    character_name TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, position)
);
CREATE TABLE library_tags (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (item_id, name)
);
CREATE TABLE library_sources (
    item_id     TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    path        TEXT    NOT NULL,
    container   TEXT    NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    bitrate     INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    edition     TEXT    NOT NULL DEFAULT '',
    plays       BOOLEAN NOT NULL DEFAULT FALSE CHECK (plays IN (0, 1)),
    PRIMARY KEY (item_id, path)
);
CREATE TABLE library_streams (
    item_id          TEXT    NOT NULL,
    path             TEXT    NOT NULL,
    stream_index     INTEGER NOT NULL,
    kind             TEXT    NOT NULL CHECK (kind IN ('video', 'audio', 'subtitle')),
    codec            TEXT    NOT NULL DEFAULT '',
    profile          TEXT    NOT NULL DEFAULT '',
    title            TEXT    NOT NULL DEFAULT '',
    is_default       BOOLEAN NOT NULL DEFAULT FALSE CHECK (is_default IN (0, 1)),
    forced           BOOLEAN NOT NULL DEFAULT FALSE CHECK (forced IN (0, 1)),
    language         TEXT    NOT NULL DEFAULT '',
    language_name    TEXT    NOT NULL DEFAULT '',
    channels         INTEGER NOT NULL DEFAULT 0,
    channel_layout   TEXT    NOT NULL DEFAULT '',
    sample_rate      INTEGER NOT NULL DEFAULT 0,
    audio_format     TEXT    NOT NULL DEFAULT '',
    width            INTEGER NOT NULL DEFAULT 0,
    height           INTEGER NOT NULL DEFAULT 0,
    frame_rate       TEXT    NOT NULL DEFAULT '',
    pixel_format     TEXT    NOT NULL DEFAULT '',
    color_transfer   TEXT    NOT NULL DEFAULT '',
    resolution       TEXT    NOT NULL DEFAULT '',
    hdr              TEXT    NOT NULL DEFAULT '',
    commentary       BOOLEAN NOT NULL DEFAULT FALSE CHECK (commentary IN (0, 1)),
    hearing_impaired BOOLEAN NOT NULL DEFAULT FALSE CHECK (hearing_impaired IN (0, 1)),
    descriptive      BOOLEAN NOT NULL DEFAULT FALSE CHECK (descriptive IN (0, 1)),
    external_ref     TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, stream_index),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_chapters (
    item_id  TEXT    NOT NULL,
    path     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms   INTEGER NOT NULL,
    title    TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_markers (
    item_id  TEXT    NOT NULL,
    path     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms   INTEGER NOT NULL,
    kind     TEXT    NOT NULL CHECK (kind IN ('cut', 'mute', 'scene', 'commercial')),
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_trickplay (
    item_id TEXT NOT NULL,
    path    TEXT NOT NULL,
    ref     TEXT NOT NULL,
    format  TEXT NOT NULL,
    PRIMARY KEY (item_id, path, ref),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE TABLE library_art (
    item_id  TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    kind     TEXT    NOT NULL CHECK (kind IN ('poster', 'fanart', 'clearlogo', 'banner', 'landscape',
        'clearart', 'discart', 'thumb', 'season-poster')),
    season   INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL,
    ref      TEXT    NOT NULL,
    PRIMARY KEY (item_id, kind, season, position)
);
CREATE TABLE library_item_images (
    item_id    TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (item_id, role, image_hash)
);
CREATE TABLE library_candidate_images (
    item_id    TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (item_id, role, image_hash)
);
CREATE TABLE library_collection_members (
    collection_id     TEXT    NOT NULL REFERENCES library_collections(id) ON DELETE CASCADE,
    inventory_item_id TEXT    NOT NULL REFERENCES library_items(id),
    position          INTEGER NOT NULL,
    PRIMARY KEY (collection_id, inventory_item_id)
);

INSERT INTO library_items
SELECT id, kind, name, sort_name, year, updated_at_ms, catalog, origin_authority, origin_key, grounded,
       searched, match_basis, match_app, match_page, needs_noticed, set_key, original_title, overview,
       premiere_date, rating, score, runtime_ms, season, episode, episode_end, series_authority,
       series_key,
       CASE WHEN EXISTS (SELECT 1 FROM library_items_old s WHERE s.id = o.series_id) THEN o.series_id END,
       name_from, year_from, overview_from, premiere_from, rating_from, score_from, runtime_from, tagline,
       trailer, unnumbered
FROM library_items_old o;
INSERT INTO library_ids SELECT * FROM library_ids_old;
INSERT INTO library_genres SELECT * FROM library_genres_old;
INSERT INTO library_studios SELECT * FROM library_studios_old;
INSERT INTO library_countries SELECT * FROM library_countries_old;
INSERT INTO library_credits SELECT * FROM library_credits_old;
INSERT INTO library_tags SELECT * FROM library_tags_old;
INSERT INTO library_sources SELECT * FROM library_sources_old;
INSERT INTO library_streams SELECT * FROM library_streams_old;
INSERT INTO library_chapters SELECT * FROM library_chapters_old;
INSERT INTO library_markers SELECT * FROM library_markers_old;
INSERT INTO library_trickplay SELECT * FROM library_trickplay_old;
INSERT INTO library_art SELECT * FROM library_art_old;
INSERT INTO library_item_images SELECT * FROM library_item_images_old;
INSERT INTO library_candidate_images SELECT * FROM library_candidate_images_old;
INSERT INTO library_collection_members SELECT * FROM library_collection_members_old;

DROP TABLE library_streams_old;
DROP TABLE library_chapters_old;
DROP TABLE library_markers_old;
DROP TABLE library_trickplay_old;
DROP TABLE library_sources_old;
DROP TABLE library_ids_old;
DROP TABLE library_genres_old;
DROP TABLE library_studios_old;
DROP TABLE library_countries_old;
DROP TABLE library_credits_old;
DROP TABLE library_tags_old;
DROP TABLE library_art_old;
DROP TABLE library_item_images_old;
DROP TABLE library_candidate_images_old;
DROP TABLE library_collection_members_old;
DROP TABLE library_items_old;

CREATE INDEX idx_library_items_catalog ON library_items (catalog);
CREATE INDEX idx_library_items_origin ON library_items (origin_authority, origin_key);
CREATE INDEX idx_library_items_series ON library_items (series_id);
CREATE INDEX idx_library_items_series_origin ON library_items (series_authority, series_key);
CREATE INDEX idx_library_items_set ON library_items (set_key);
CREATE INDEX idx_library_items_sort ON library_items (kind, sort_name);
CREATE INDEX idx_library_items_unnumbered ON library_items (series_id) WHERE unnumbered;
CREATE INDEX idx_library_ids_value ON library_ids (namespace, value);
CREATE INDEX idx_library_genres_name ON library_genres (name);
CREATE INDEX idx_library_studios_name ON library_studios (name);
CREATE INDEX idx_library_countries_name ON library_countries (name);
CREATE INDEX idx_library_credits_person ON library_credits (person_id, role);
CREATE INDEX idx_library_tags_name ON library_tags (name);
CREATE INDEX idx_library_sources_path ON library_sources (path);
CREATE INDEX idx_library_streams_language ON library_streams (kind, language);
CREATE INDEX idx_library_item_images_image ON library_item_images (image_hash);
CREATE INDEX idx_library_candidate_images_image ON library_candidate_images (image_hash);
CREATE INDEX idx_library_collection_members_item ON library_collection_members (inventory_item_id);

-- library_files ----------------------------------------------------------------------------------
ALTER TABLE library_files RENAME TO library_files_old;
CREATE TABLE library_files (
    path                TEXT    PRIMARY KEY,
    root                TEXT    NOT NULL,
    size_bytes          INTEGER NOT NULL,
    modified_ns         INTEGER NOT NULL,
    identity            TEXT    NOT NULL DEFAULT '',
    kind                TEXT    NOT NULL DEFAULT '' CHECK (kind IN ('', 'movie', 'episode', 'series')),
    header              TEXT    NOT NULL DEFAULT 'pending',
    problem             TEXT    NOT NULL DEFAULT '',
    held                BOOLEAN NOT NULL DEFAULT FALSE CHECK (held IN (0, 1)),
    seen_at_ms          INTEGER NOT NULL,
    missing_at_ms       INTEGER NOT NULL DEFAULT 0,
    inventory_item_id   TEXT    REFERENCES library_items(id),
    inventory_series_id TEXT    REFERENCES library_items(id)
);
INSERT INTO library_files
SELECT path, root, size_bytes, modified_ns, identity, kind, header, problem, held, seen_at_ms, missing_at_ms,
       CASE WHEN EXISTS (SELECT 1 FROM library_items i WHERE i.id = f.inventory_item_id) THEN f.inventory_item_id END,
       CASE WHEN EXISTS (SELECT 1 FROM library_items i WHERE i.id = f.inventory_series_id) THEN f.inventory_series_id END
FROM library_files_old f;
DROP TABLE library_files_old;
CREATE INDEX idx_library_files_inventory_item ON library_files (inventory_item_id);
CREATE INDEX idx_library_files_inventory_series ON library_files (inventory_series_id);
CREATE INDEX idx_library_files_root ON library_files (root);

-- library_candidates -----------------------------------------------------------------------------
ALTER TABLE library_candidates RENAME TO library_candidates_old;
CREATE TABLE library_candidates (
    inventory_item_id TEXT    NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    rank              INTEGER NOT NULL,
    kind              TEXT    NOT NULL CHECK (kind IN ('movie', 'series')),
    external_id       TEXT    NOT NULL,
    name              TEXT    NOT NULL,
    year              INTEGER NOT NULL DEFAULT 0,
    overview          TEXT    NOT NULL DEFAULT '',
    poster            TEXT    NOT NULL DEFAULT '',
    name_match        BOOLEAN NOT NULL DEFAULT FALSE CHECK (name_match IN (0, 1)),
    year_match        BOOLEAN NOT NULL DEFAULT FALSE CHECK (year_match IN (0, 1)),
    length_match      BOOLEAN NOT NULL DEFAULT FALSE CHECK (length_match IN (0, 1)),
    namespace         TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (inventory_item_id, rank)
);
INSERT INTO library_candidates
SELECT * FROM library_candidates_old c
WHERE EXISTS (SELECT 1 FROM library_items i WHERE i.id = c.inventory_item_id);
DROP TABLE library_candidates_old;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
