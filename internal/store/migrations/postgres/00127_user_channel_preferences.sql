-- +goose Up
-- Per-person channel lists for the guide's Favourites and Recent filters (#1666). Keyed by the user,
-- not the device: a paired TV acts as the person who paired it, so a channel starred on the TV shows
-- on that person's phone. Both lists die with their user or their channel.
CREATE TABLE user_favourite_channels (
    user_id    TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    added_at   BIGINT NOT NULL CHECK (added_at > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
CREATE INDEX user_favourite_channels_channel ON user_favourite_channels (channel_id);

-- One row per (user, channel), holding the latest tune; the store keeps only the newest few per user.
CREATE TABLE user_recent_channels (
    user_id    TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    tuned_at   BIGINT NOT NULL CHECK (tuned_at > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
CREATE INDEX user_recent_channels_channel ON user_recent_channels (channel_id);
CREATE INDEX user_recent_channels_user_tuned ON user_recent_channels (user_id, tuned_at DESC);

-- +goose Down
SELECT 1;
