-- +goose Up
-- #3855 (decision 12 of #3838): a download that fails retries on its own, three attempts over a day,
-- then it rests 30 days. Until now only a count of failures per run existed, so nothing knew which
-- item had failed or when it was last tried.
--
-- One row per item that has not downloaded yet, deleted when it does. `failures` counts attempts
-- that failed; a wait for a temporary cause (no room, a source that cannot be reached) leaves it
-- alone. `retry_at` is the earliest the item is tried again, in Unix seconds; a pass that picks the
-- item up moves it forward first, so a restart mid-download does not start another at once.
-- `item_key` is the provider's item id when it has one, else the URL.
CREATE TABLE filler_download_failures (
    item_key        TEXT PRIMARY KEY,
    provider        TEXT NOT NULL DEFAULT '',
    source_id       TEXT NOT NULL DEFAULT '',
    remote_id       TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL,
    duration_ms     INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    height          INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    gap             TEXT NOT NULL DEFAULT '',
    failures        INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
    first_failed_at INTEGER NOT NULL DEFAULT 0,
    last_failed_at  INTEGER NOT NULL DEFAULT 0,
    retry_at        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_filler_download_failures_retry ON filler_download_failures(retry_at, item_key);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
