-- +goose Up
-- #4333 (decisions 8 and 32 of #4140): Postgres mirror of sqlite 00243; the full reasoning lives there.
CREATE TABLE growth_reviews (
    channel_id  TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    title_key   TEXT   NOT NULL,
    line        TEXT   NOT NULL,
    verdict     TEXT   NOT NULL CHECK (verdict IN ('fits', 'does_not_fit')),
    reason      TEXT   NOT NULL DEFAULT '',
    model       TEXT   NOT NULL DEFAULT '',
    reviewed_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (channel_id, title_key)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
