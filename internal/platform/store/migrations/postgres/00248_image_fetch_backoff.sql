-- +goose Up
-- A download that fails backs off (#4476): each failed attempt is counted and timed, and the fetch
-- queue takes the image again only once fetch_retry_at has passed, so an image that keeps failing
-- costs one attempt a day instead of holding a slot of every pass. A download that lands clears
-- all three.
ALTER TABLE images ADD COLUMN fetch_failures INTEGER NOT NULL DEFAULT 0 CHECK (fetch_failures >= 0);
ALTER TABLE images ADD COLUMN fetch_failed_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE images ADD COLUMN fetch_retry_at BIGINT NOT NULL DEFAULT 0;

-- The fetch queue is first come, first served by when an image joined it: its adoption, or the
-- expiry purge's requeue. created_at would put a requeued image ahead of everything adopted since.
-- Rows already waiting joined when they were created.
ALTER TABLE images ADD COLUMN fetch_queued_at BIGINT NOT NULL DEFAULT 0;
UPDATE images SET fetch_queued_at = created_at;

-- Serves the queue's read in order: the equality on origin and origin_fetched_at, then the queue
-- order, so a pass reads the front of the queue rather than sorting every waiting row.
CREATE INDEX idx_images_fetch_queue ON images (origin, origin_fetched_at, fetch_queued_at, hash);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
