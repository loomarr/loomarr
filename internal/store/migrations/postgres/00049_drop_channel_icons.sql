-- +goose Up
-- V52 phase 8 (§22). Postgres mirror of the sqlite 00049; the full reasoning lives there.
--
-- In short: `channel_icons` stored an uploaded icon as a blob keyed by CHANNEL, served by
-- GET /v1/channels/{id}/icon. Phase 5 moved uploads to the image service and left this as the
-- migration window for icons uploaded before it; the window closes here. No backfill — this
-- project has no production installs, so there is no pre-phase-5 data to carry, and a job written
-- to migrate data that does not exist is debt rather than safety.
--
-- ⚠ DeleteChannel now deletes IMAGE REFS instead of a row here, which fixed a leak: phase 5 moved
-- the bytes but left the delete removing the old blob, so the ref survived and the GC never saw
-- the icon as an orphan.
DROP TABLE IF EXISTS channel_icons;

-- +goose Down
-- Recreated EMPTY. The bytes are not restorable — they live in the image service, addressed by
-- content hash, with nothing recording which channel-keyed row they came from.
CREATE TABLE IF NOT EXISTS channel_icons (
    channel_id   TEXT PRIMARY KEY,
    content_type TEXT NOT NULL,
    bytes        BYTEA NOT NULL,
    updated_at   BIGINT NOT NULL DEFAULT 0
);
