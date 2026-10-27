-- +goose Up
-- Commercial rotation is household-wide (#2017): a clip seen on any channel rests on every channel.
-- One row per watched airing replaces the per-(channel, clip) aggregate of 00066. A break's
-- snapshot must not change while it airs, and with several channels that needs each airing's
-- recorded_at: a viewer tuning in mid-clip on another channel records an airing that started
-- minutes ago, after the break began. Playout keeps the newest few airings per clip, so the log
-- stays bounded like the aggregate it replaces. No foreign key to channels: an ad the household saw
-- on a channel it later deleted was still seen.
CREATE TABLE filler_airings (
    channel_id TEXT NOT NULL,
    clip_hash TEXT NOT NULL,
    aired_at BIGINT NOT NULL,
    recorded_at BIGINT NOT NULL,
    PRIMARY KEY (channel_id, clip_hash, aired_at)
);

CREATE INDEX idx_filler_airings_clip ON filler_airings(clip_hash, aired_at);

-- The aggregate held each channel's latest airing and the one before it; both are long settled.
INSERT INTO filler_airings (channel_id, clip_hash, aired_at, recorded_at)
    SELECT channel_id, clip_hash, last_played_at, last_played_at FROM filler_exposures WHERE last_played_at > 0;
INSERT INTO filler_airings (channel_id, clip_hash, aired_at, recorded_at)
    SELECT channel_id, clip_hash, previous_played_at, previous_played_at FROM filler_exposures
    WHERE previous_played_at > 0 AND previous_played_at <> last_played_at;

DROP TABLE filler_exposures;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
