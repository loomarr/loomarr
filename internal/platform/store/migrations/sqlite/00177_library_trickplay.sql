-- +goose Up
-- Trickplay references (#2151 decisions 45, 61). One row per trickplay file beside a title's
-- file: Emby's "<name>.bif" seek thumbnails, referenced and never imported. Rewritten with each
-- inventory write (decision 43) and filled by the write-version bump (decision 55).
CREATE TABLE library_trickplay (
    item_id TEXT NOT NULL,
    path TEXT NOT NULL,
    ref TEXT NOT NULL,
    format TEXT NOT NULL,
    PRIMARY KEY (item_id, path, ref),
    FOREIGN KEY (item_id, path) REFERENCES library_sources(item_id, path) ON DELETE CASCADE
);

-- +goose Down
-- No down: forward-only (§16).
