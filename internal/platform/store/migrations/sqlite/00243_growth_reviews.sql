-- +goose Up
-- #4333 (decisions 8 and 32 of #4140): the model reviews, in the background, the titles growth adds
-- to a channel and the unclear fits it holds back. A verdict is kept per channel and title so it is
-- asked once, and asked again only when the channel's line changes: `line` is the line the verdict
-- was given against, in the words growth describes it.
--
-- `verdict` is the model's answer from a fixed choice. A title it says does not fit, which growth
-- added, is taken off the lineup (and declined, so the nightly fill leaves it out); an unclear title
-- it says fits may join. `reason` is the model's few words, shown with the title.
CREATE TABLE growth_reviews (
    channel_id  TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    title_key   TEXT    NOT NULL,
    line        TEXT    NOT NULL,
    verdict     TEXT    NOT NULL CHECK (verdict IN ('fits', 'does_not_fit')),
    reason      TEXT    NOT NULL DEFAULT '',
    model       TEXT    NOT NULL DEFAULT '',
    reviewed_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (channel_id, title_key)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
