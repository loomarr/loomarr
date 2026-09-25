-- +goose Up
-- #1456: the media server's OWN file path for a library item, remembered so playout can resolve a
-- scheduled item's input with no media-server request at airtime (a channel keeps airing through a
-- media-server outage). The SERVER path is stored, never the mapped local one: `library.path_map`
-- is applied at read time, so an edit needs no rebuild. A stale row is harmless — a path that no
-- longer stats falls back to a lookup, which rewrites it.
CREATE TABLE library_item_paths (
    item_id     TEXT   NOT NULL PRIMARY KEY,
    server_path TEXT   NOT NULL,
    updated_at  BIGINT NOT NULL DEFAULT 0
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
