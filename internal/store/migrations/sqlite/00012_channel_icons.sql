-- +goose Up
-- Uploaded channel icons (the icon feature — user-upload source). A channel's icon can come
-- from a TMDB poster URL (stored inline in channels.logo, no bytes here) OR an upload; an
-- upload's bytes live here so they survive a restart and ride the §16 backup, with NO new
-- filesystem/asset-dir config (icons are small — a few hundred KB). The channel's logo column
-- then holds this row's serve URL (/v1/channels/{id}/icon), which Tunarr fetches directly.
--
-- One icon per channel (the channel id is the PK), replaced on re-upload. content_type is the
-- upload's MIME (image/png|jpeg|webp|svg+xml) so the serve endpoint sends the right header.
-- updated_at drives a cache-busting query param so a re-upload isn't served stale.
--
-- Forward-only (§16).
CREATE TABLE channel_icons (
    channel_id   TEXT PRIMARY KEY,
    content_type TEXT NOT NULL,
    bytes        BLOB NOT NULL,
    updated_at   INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE channel_icons;
