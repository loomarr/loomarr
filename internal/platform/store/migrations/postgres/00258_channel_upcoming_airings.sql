-- +goose Up
-- Each internal-playout channel's programmes for the next 48 hours, replaced whole at every
-- reconcile, so a title's page can say when it is next on in one indexed read (#4476,
-- scheduling.md "Upcoming airings"). A movie is found by its item id, a series by its title key.
CREATE TABLE channel_upcoming_airings (
    channel_id      TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    starts_at_ms    BIGINT  NOT NULL,
    ends_at_ms      BIGINT  NOT NULL,
    library_item_id TEXT    NOT NULL,
    title_key       TEXT    NOT NULL,
    forecast        BOOLEAN NOT NULL,
    revision        BIGINT  NOT NULL,
    PRIMARY KEY (channel_id, starts_at_ms)
);
CREATE INDEX idx_channel_upcoming_airings_item ON channel_upcoming_airings (library_item_id, ends_at_ms);
CREATE INDEX idx_channel_upcoming_airings_title ON channel_upcoming_airings (title_key, ends_at_ms);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
