-- +goose Up
-- Library → Collections (#2151 step 12, decision 28): the household's own collections, each an
-- ordered list of library titles.
--
-- `imported_from` names the media server a collection was brought over from at cutover ('' made
-- in Loomarr). A member is a `library_items` id; one whose title has left the library is skipped
-- when read, and kept so the title comes back in its place.
CREATE TABLE library_collections (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    imported_from TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

CREATE TABLE library_collection_members (
    collection_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (collection_id, item_id)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
