-- +goose Up
-- Per-channel filler exposure is the durable input to anti-repeat rotation (§10 V58).
-- One aggregate row per (channel, clip) survives catalog removal/re-add without an unbounded
-- airing log; millisecond precision lets planning exclude the break currently going to air.
CREATE TABLE filler_exposures (
    channel_id TEXT NOT NULL,
    clip_hash TEXT NOT NULL,
    play_count INTEGER NOT NULL DEFAULT 0,
    last_played_at INTEGER NOT NULL DEFAULT 0,
    previous_played_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (channel_id, clip_hash),
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

CREATE INDEX idx_filler_exposures_channel ON filler_exposures(channel_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
