-- +goose Up
-- Airing history becomes a short LOG per (channel, episode/film), with the time each row was
-- written (#1674).
--
-- WHY. 00019 kept one upserted row per (channel, provisioning key). Two things went wrong:
--
--   1. A series key covers every episode, so one tune-in marked the whole show as "just aired"
--      and recency pushed all of its episodes to the back of the deck.
--   2. Placement read the live row on every reconcile, so a tune-in re-sorted the deck in the
--      middle of the window already on air: the same wall-clock instant mapped to a different
--      programme and the guide stopped matching the picture.
--
-- The fix reads history AS OF the start of the window being arranged: for each unit, the latest
-- airing whose row was written before that instant. An upserted "last airing" cannot answer that
-- once the unit airs again inside the window, and a row written late (a viewer tuning in to a
-- programme that started before the boundary) must not count either — hence one row per airing
-- plus `recorded_at`. RecordAiring prunes rows older than the retention horizon except the newest
-- of them, so a unit keeps at most its recent airings plus one: still bounded by lineup size.
--
-- Existing rows carry over as a single airing of the item that last streamed, recorded when it
-- aired (the best the old shape knows). Rows without an item id identify no unit and are dropped.
CREATE TABLE airings_v125 (
    channel_id      TEXT   NOT NULL,
    library_item_id TEXT   NOT NULL,           -- the unit that aired (episode or film)
    aired_at        BIGINT NOT NULL,           -- unix seconds the programme started
    recorded_at     BIGINT NOT NULL,           -- unix seconds the row was first written
    key             TEXT   NOT NULL DEFAULT '', -- provision.Key it belongs to (diagnostics)
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);

INSERT INTO airings_v125 (channel_id, library_item_id, aired_at, recorded_at, key)
SELECT channel_id, library_item_id, aired_at, aired_at, key
FROM airings
WHERE library_item_id <> '';

DROP TABLE airings;
ALTER TABLE airings_v125 RENAME TO airings;
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
