-- +goose Up
-- The id map for the cutover (#2151 step 15a): each media-server item id Loomarr may have stored,
-- and the library item it names. Movies and episodes pair by file path through the derived path
-- map; a series pairs through its episodes. An id that does not pair keeps an empty item_id and the
-- reason. Replaced whole on each pass while the media server still lists the catalog.
CREATE TABLE library_id_map (
    media_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    item_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    mapped_at_ms INTEGER NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
