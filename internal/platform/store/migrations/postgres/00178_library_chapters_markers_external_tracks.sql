-- +goose Up
-- Header extras and sidecars (#2151 decisions 45, 46, 61). A track says whether it is commentary,
-- for the deaf and hard of hearing (SDH) or describes the picture, and an external track names the
-- file it lives in (external_ref; '' for a track inside the media file). A file's chapters and its
-- .edl markers are rows of their own. Rewritten with each inventory write (decision 43) and filled
-- by the write-version bump (decision 55).
ALTER TABLE library_streams ADD COLUMN commentary BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE library_streams ADD COLUMN hearing_impaired BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE library_streams ADD COLUMN descriptive BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE library_streams ADD COLUMN external_ref TEXT NOT NULL DEFAULT '';

CREATE TABLE library_chapters (
    item_id TEXT NOT NULL,
    path TEXT NOT NULL,
    position INTEGER NOT NULL,
    start_ms BIGINT NOT NULL,
    end_ms BIGINT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);

CREATE TABLE library_markers (
    item_id TEXT NOT NULL,
    path TEXT NOT NULL,
    position INTEGER NOT NULL,
    start_ms BIGINT NOT NULL,
    end_ms BIGINT NOT NULL,
    kind TEXT NOT NULL,
    PRIMARY KEY (item_id, path, position),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);

-- +goose Down
-- No down: forward-only (§16).
