-- +goose Up
-- Schema to a 10 (#2793, item 2): references move out of JSON. A channel's lineup was a JSON list
-- in channels.lineup_json, so the title each entry names could not be indexed or checked, and the
-- title purge had to load every channel to learn which titles a lineup still names. Each entry is
-- now a row: its title key a column, its other attributes (runtime, season window, the rating,
-- genres and year the rules filter on) the entry document.
--
-- title_key is not yet a foreign key: a title added to a lineup by hand has no titles row until
-- something asks for it. Box-set ids stay in the entry until the #2151 Library id cutover settles.
CREATE TABLE channel_lineup (
    channel_id TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    title_key  TEXT    NOT NULL,
    entry      TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(entry)),
    PRIMARY KEY (channel_id, position)
);
CREATE INDEX idx_channel_lineup_title ON channel_lineup (title_key);

INSERT INTO channel_lineup (channel_id, position, title_key, entry)
SELECT c.id, CAST(e.key AS INTEGER), json_extract(e.value, '$.Key'), json_remove(e.value, '$.Key')
FROM channels c, json_each(c.lineup_json) e;

ALTER TABLE channels DROP COLUMN lineup_json;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
