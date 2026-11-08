-- +goose Up
-- Postgres mirror of sqlite 00186: the scheduled ledger, what a channel's schedule put on air
-- whether or not anyone watched, kept apart from `airings` (#1677).
CREATE TABLE scheduled_airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
CREATE INDEX idx_scheduled_airings_channel ON scheduled_airings (channel_id, recorded_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
