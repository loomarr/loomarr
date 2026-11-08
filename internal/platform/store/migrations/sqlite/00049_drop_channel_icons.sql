-- +goose Up
-- V52 phase 8 (§22): the `channel_icons` table retires.
--
-- It stored an uploaded channel icon as a BLOB keyed by channel id, served by
-- GET /v1/channels/{id}/icon. That shape predates the image service and is the thing §22 exists to
-- replace: bytes in the database rather than on disk, addressed by CHANNEL rather than by CONTENT,
-- with a `?v=` cache-bust bolted on precisely because the URL could not change when the bytes did.
--
-- V52 phase 5 moved uploads to the image service and left this table as the migration window for
-- icons uploaded before it. The window is what closes here.
--
-- ⚠ **No backfill, deliberately, and the reason is a fact about this project rather than a
-- judgement about migrations.** Loomarr has no production installs — nothing has ever run long
-- enough to accumulate icons that predate phase 5. A backfill job would therefore be code written
-- to migrate data that does not exist, which is exactly the debt this repository would rather not
-- carry: it would need its own gate, its own tests, and a reader in five years would have to work
-- out whether it still mattered. If that changes before release, the honest fix is to restore the
-- window, not to keep a dead job.
--
-- ⚠ **`DeleteChannel` no longer deletes from here — it deletes IMAGE REFS**, and that swap fixed a
-- leak rather than merely renaming a cleanup. Phase 5 moved the bytes but left the delete removing
-- the old blob, so a deleted channel's `image_refs` row survived it. A ref is what tells the GC an
-- image is still in use, so the icon was never orphaned and never collected. See the comment on
-- DeleteChannel and the ChannelDeleteDropsImageRefs conformance subtest.
DROP TABLE IF EXISTS channel_icons;

-- +goose Down
-- Recreated empty. The bytes are NOT restorable — they live in the image service now, addressed by
-- content hash, and nothing records which channel-keyed row they came from. A down migration that
-- silently produced an empty table while implying otherwise would be worse than one that says so.
CREATE TABLE IF NOT EXISTS channel_icons (
    channel_id   TEXT PRIMARY KEY,
    content_type TEXT    NOT NULL,
    bytes        BLOB    NOT NULL,
    updated_at   INTEGER NOT NULL DEFAULT 0
);
