-- +goose Up
-- #3855 (decision 12 of #3838): Postgres mirror of sqlite 00242; the full reasoning lives there.
CREATE TABLE filler_download_failures (
    item_key        TEXT PRIMARY KEY,
    provider        TEXT NOT NULL DEFAULT '',
    source_id       TEXT NOT NULL DEFAULT '',
    remote_id       TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL,
    duration_ms     BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    height          BIGINT NOT NULL DEFAULT 0 CHECK (height >= 0),
    gap             TEXT NOT NULL DEFAULT '',
    failures        BIGINT NOT NULL DEFAULT 0 CHECK (failures >= 0),
    first_failed_at BIGINT NOT NULL DEFAULT 0,
    last_failed_at  BIGINT NOT NULL DEFAULT 0,
    retry_at        BIGINT NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_filler_download_failures_retry ON filler_download_failures(retry_at, item_key);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
