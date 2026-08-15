-- +goose Up
-- Postgres mirror of sqlite 00053. Requeue clip hover artwork that was adopted while image ingest
-- preserved animated WebP bytes but failed to record that they carried motion. The adoption job
-- inspects the bytes; this migration deliberately does not assume every preview is animated.
UPDATE clips
   SET hover_image_hash = ''
 WHERE preview <> ''
   AND hover_image_hash <> ''
   AND EXISTS (
       SELECT 1
         FROM images
        WHERE images.hash = clips.hover_image_hash
          AND images.mime = 'image/webp'
          AND images.animated = FALSE
   );

-- +goose Down
-- Re-adoption is content-addressed and idempotent; there is no honest hash to restore here.
SELECT 1;
