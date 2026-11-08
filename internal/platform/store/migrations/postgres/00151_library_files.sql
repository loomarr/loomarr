-- +goose Up
-- The scanner's file table (#2151 step 5): one row per media file found under an in-use root.
--
-- `size` and `modified_ns` are the file's last walked stat; together they are its revision, so an
-- unchanged file is never opened again. `identity` is a digest of what said what the file is (the
-- arr record, ids in its names, its NFO sidecar's stat) when it was last written to the inventory.
-- `kind` is what the file was last written as: `movie`, `episode` or `extra` ('' for a problem).
-- `header` is `pending`, `read` or `failed`: whether its container header has been read. `problem`
-- names why a file is skipped (`disc_image`, `identity_conflict`), '' for none. `held` marks a file
-- a walk no longer found while that walk's removals wait for the maintainer (decision 33).
-- `missing_at` is when a walk stopped finding it (unix ms, 0 = present).
--
-- `library_roots.scanned_at` is when a walk of the root last completed (unix ms, 0 = never).
CREATE TABLE library_files (
    path TEXT PRIMARY KEY,
    root TEXT NOT NULL,
    size BIGINT NOT NULL,
    modified_ns BIGINT NOT NULL,
    identity TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT '',
    header TEXT NOT NULL DEFAULT 'pending',
    problem TEXT NOT NULL DEFAULT '',
    held BOOLEAN NOT NULL DEFAULT FALSE,
    seen_at BIGINT NOT NULL,
    missing_at BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX library_files_root ON library_files (root);

ALTER TABLE library_roots ADD COLUMN scanned_at BIGINT NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
