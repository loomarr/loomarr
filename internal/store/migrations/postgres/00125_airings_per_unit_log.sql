-- +goose Up
-- Postgres mirror of sqlite 00125: airing history becomes a short log per (channel, episode/film)
-- with the time each row was written, so placement can read it as of a window's start (#1674).
CREATE TABLE airings_v125 (
    channel_id      TEXT   NOT NULL,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);

INSERT INTO airings_v125 (channel_id, library_item_id, aired_at, recorded_at, key)
SELECT channel_id, library_item_id, aired_at, aired_at, key
FROM airings
WHERE library_item_id <> '';

DROP TABLE airings;
ALTER TABLE airings_v125 RENAME TO airings;
ALTER INDEX airings_v125_pkey RENAME TO airings_pkey;
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
