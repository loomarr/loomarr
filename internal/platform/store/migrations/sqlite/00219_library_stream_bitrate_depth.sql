-- +goose Up
-- #3228 (#2151 decision 46): each track's own bitrate, as its container states or its sample
-- table measures it, and its bit depth. 0 is "the file says nothing". header.Version 4 reads every
-- file's header once more, so existing rows fill on the next pass.
ALTER TABLE library_streams ADD COLUMN bitrate INTEGER NOT NULL DEFAULT 0;
ALTER TABLE library_streams ADD COLUMN bit_depth INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
