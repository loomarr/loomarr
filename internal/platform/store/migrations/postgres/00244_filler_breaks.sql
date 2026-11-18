-- +goose Up
-- #3838 step 2: Postgres mirror of sqlite 00244; the full reasoning lives there.
CREATE TABLE filler_breaks (
    channel_id  TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    start_ms    BIGINT NOT NULL,
    ceiling_ms  BIGINT NOT NULL CHECK (ceiling_ms > 0),
    length_ms   BIGINT NOT NULL CHECK (length_ms >= 0),
    pod_json    JSONB  NOT NULL,
    planned_at  BIGINT NOT NULL,
    PRIMARY KEY (channel_id, start_ms)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
