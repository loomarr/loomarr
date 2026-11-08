-- +goose Up
-- Rich library data, files and tracks (#2151 decisions 41, 44, 45; ADR 0047). One row per file of
-- a title, with the copy that plays marked (`plays`, decision 37) and its edition ('' until
-- editions, decision 54); one row per track, by its stream index, with the raw values and labels
-- computed once (decision 44): resolution (by width too), HDR type, audio format, language code
-- and display name. Artwork is one row per reference, season posters by `season`. Nothing is
-- copied onto the title. All are rewritten with each inventory write (decision 43) and filled by
-- the write-version bump (decision 55).
CREATE TABLE library_sources (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    container TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    bitrate BIGINT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    edition TEXT NOT NULL DEFAULT '',
    plays BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (item_id, path)
);
CREATE INDEX idx_library_sources_path ON library_sources (path);

CREATE TABLE library_streams (
    item_id TEXT NOT NULL,
    path TEXT NOT NULL,
    stream_index INTEGER NOT NULL,
    kind TEXT NOT NULL,
    codec TEXT NOT NULL DEFAULT '',
    profile TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    forced BOOLEAN NOT NULL DEFAULT FALSE,
    language TEXT NOT NULL DEFAULT '',
    language_name TEXT NOT NULL DEFAULT '',
    channels INTEGER NOT NULL DEFAULT 0,
    channel_layout TEXT NOT NULL DEFAULT '',
    sample_rate INTEGER NOT NULL DEFAULT 0,
    audio_format TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    frame_rate TEXT NOT NULL DEFAULT '',
    pixel_format TEXT NOT NULL DEFAULT '',
    color_transfer TEXT NOT NULL DEFAULT '',
    resolution TEXT NOT NULL DEFAULT '',
    hdr TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, stream_index),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);
CREATE INDEX idx_library_streams_language ON library_streams (kind, language);

CREATE TABLE library_art (
    item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    season INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL,
    ref TEXT NOT NULL,
    PRIMARY KEY (item_id, kind, season, position)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
