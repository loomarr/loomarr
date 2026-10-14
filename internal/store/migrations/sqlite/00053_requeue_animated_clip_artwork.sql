-- +goose Up
-- Repair hover loops adopted before animated WebP detection was wired into image ingest.
--
-- The image service always copied the ORIGINAL bytes intact, so no media was lost: the stored
-- .webp still has every frame. Only `images.animated` was false, which sent rendition requests
-- through the still-image encoder and flattened the response to frame zero. Clear the clip's
-- pointer so the normal idempotent artwork-adoption job re-ingests those preserved bytes and
-- derives the flag from the RIFF container.
--
-- ⚠ Do not set `images.animated = 1` here. `preview` means "intended hover loop", not "these bytes
-- definitely contain motion"; the renderer may legitimately have produced a static fallback.
-- Re-ingest is what keeps the bytes, rather than this migration's assumption, authoritative.
UPDATE clips
   SET hover_image_hash = ''
 WHERE preview <> ''
   AND hover_image_hash <> ''
   AND EXISTS (
       SELECT 1
         FROM images
        WHERE images.hash = clips.hover_image_hash
          AND images.mime = 'image/webp'
          AND images.animated = 0
   );

-- +goose Down
-- The previous identity is deliberately not reconstructed: the Up migration queues an idempotent
-- re-adoption of the same content-addressed bytes, and guessing the hash during rollback would be
-- a second implementation of that job.
SELECT 1;
