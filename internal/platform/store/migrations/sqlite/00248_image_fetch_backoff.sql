-- +goose Up
-- A download that fails backs off (#4476): each failed attempt is counted and timed, and the fetch
-- queue takes the image again only once fetch_retry_at has passed, so an image that keeps failing
-- costs one attempt a day instead of holding a slot of every pass. A download that lands clears
-- all three.
ALTER TABLE images ADD COLUMN fetch_failures INTEGER NOT NULL DEFAULT 0 CHECK (fetch_failures >= 0);
ALTER TABLE images ADD COLUMN fetch_failed_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE images ADD COLUMN fetch_retry_at INTEGER NOT NULL DEFAULT 0;

-- The fetch queue is served by priority, then by when an image joined it. fetch_priority 0 is
-- artwork a surface just adopted, 1 an image the expiry purge requeued for a refresh, 2 artwork
-- adopted in bulk to fetch last (episode stills), so new artwork never waits behind a mass
-- requeue. Rows already waiting are adoptions that joined when they were created.
ALTER TABLE images ADD COLUMN fetch_priority INTEGER NOT NULL DEFAULT 0 CHECK (fetch_priority IN (0, 1, 2));
ALTER TABLE images ADD COLUMN fetch_queued_at INTEGER NOT NULL DEFAULT 0;
UPDATE images SET fetch_queued_at = created_at;

-- Serves the queue's read in order: the equalities on origin and origin_fetched_at, then the
-- queue order, so a pass reads the front of the queue rather than sorting every waiting row. Its
-- (origin, origin_fetched_at) prefix serves what idx_images_origin_fetched did, so that one goes.
CREATE INDEX idx_images_fetch_queue ON images (origin, origin_fetched_at, fetch_priority, fetch_queued_at, hash);
DROP INDEX IF EXISTS idx_images_origin_fetched;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
