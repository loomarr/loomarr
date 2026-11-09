-- +goose Up
-- The watch history (#3010): one row per person watching one programme on one channel on one
-- device, from the first playlist poll to the last. Admins read it; it is kept until an admin
-- clears it. channel_id has no foreign key, on purpose: the history outlives a deleted channel,
-- and channel_name keeps what it was called. A person's history goes with them.
CREATE TABLE viewings (
    id           TEXT   PRIMARY KEY,
    user_id      TEXT   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device       TEXT   NOT NULL DEFAULT '',
    channel_id   TEXT   NOT NULL,
    channel_name TEXT   NOT NULL DEFAULT '',
    programme    TEXT   NOT NULL DEFAULT '',
    started_at   BIGINT NOT NULL CHECK (started_at > 0),
    ended_at     BIGINT NOT NULL,
    CHECK (ended_at >= started_at)
);
CREATE INDEX idx_viewings_started ON viewings (started_at);
CREATE INDEX idx_viewings_user_started ON viewings (user_id, started_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
