-- +goose Up
-- The scheduled ledger (#1677): one row each time a channel's schedule put an episode or film on
-- air, whether or not anyone watched. `airings` keeps what was actually streamed (the watched
-- signal); "aired" means both, as separate signals (#1678 decision 1). With nobody watching,
-- `airings` stays empty and a new window could not tell what the window before it aired, so a film
-- could come back within hours. Same shape and cutoff as `airings`: read as of a window's start.
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
