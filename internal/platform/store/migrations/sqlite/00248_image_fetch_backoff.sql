-- +goose Up
-- A download that fails backs off (#4476): each failed attempt is counted and timed, and the fetch
-- queue takes the image again only once fetch_retry_at has passed, so an image that keeps failing
-- costs one attempt a day instead of holding a slot of every pass. A download that lands clears
-- all three.
ALTER TABLE images ADD COLUMN fetch_failures INTEGER NOT NULL DEFAULT 0 CHECK (fetch_failures >= 0);
ALTER TABLE images ADD COLUMN fetch_failed_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE images ADD COLUMN fetch_retry_at INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
