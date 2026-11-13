-- +goose Up
-- Schema to a 10 (#2793, item 1): the library items a title and an airing name are foreign keys.
--
--   - titles.library_id, the library item a title is, references library_items. It used '' for
--     "not in the library", which no item has as its id, so it becomes NULL;
--   - airings.library_item_id and scheduled_airings.library_item_id, what aired or is scheduled,
--     reference library_items.
--
-- Neither deletes with the item (no ON DELETE action): the catalog's retention already keeps a
-- library row while any stored record names it (PurgeSpentCatalog, #2528), and the keys make the
-- database hold that too.
--
-- The #2151 cutover moved every stored media-server id to its library id where the media server
-- still knew it. What is left names media the server no longer has, so it can never be a library
-- item: such a title has no library item (NULL), and such an airing is dropped. Every live airing
-- already names a library item; the dropped ones are older history of titles that are gone.
--
-- titles takes the key in place (the 00211 pattern). airings and scheduled_airings rebuild, since
-- the column is part of their primary key.

-- titles ---------------------------------------------------------------------------------------
ALTER TABLE titles ADD COLUMN library_item TEXT REFERENCES library_items(id);
UPDATE titles SET library_item = (SELECT i.id FROM library_items i WHERE i.id = titles.library_id);
DROP INDEX idx_titles_library_id;
ALTER TABLE titles DROP COLUMN library_id;
ALTER TABLE titles RENAME COLUMN library_item TO library_id;
CREATE INDEX idx_titles_library_id ON titles (library_id);

-- airings and scheduled_airings ----------------------------------------------------------------
ALTER TABLE airings RENAME TO airings_old;
ALTER TABLE scheduled_airings RENAME TO scheduled_airings_old;
CREATE TABLE airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL REFERENCES library_items(id),
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
CREATE TABLE scheduled_airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL REFERENCES library_items(id),
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at, key)
SELECT channel_id, library_item_id, aired_at, recorded_at, key FROM airings_old a
WHERE EXISTS (SELECT 1 FROM library_items i WHERE i.id = a.library_item_id);
INSERT INTO scheduled_airings (channel_id, library_item_id, aired_at, recorded_at, key)
SELECT channel_id, library_item_id, aired_at, recorded_at, key FROM scheduled_airings_old a
WHERE EXISTS (SELECT 1 FROM library_items i WHERE i.id = a.library_item_id);
DROP TABLE airings_old;
DROP TABLE scheduled_airings_old;
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);
CREATE INDEX idx_airings_library_item ON airings (library_item_id);
CREATE INDEX idx_scheduled_airings_channel ON scheduled_airings (channel_id, recorded_at);
CREATE INDEX idx_scheduled_airings_library_item ON scheduled_airings (library_item_id);

-- channel_planned_items and channel_scope_collections ---------------------------------------------
-- Item 2: the library ids a channel's JSON names live in tables too, written with the channel, so
-- the keys hold them and nothing decodes channels to find them. desired_json's programme slots name
-- library items; the policy's scope and its rules name collections. Neither table has an ON DELETE
-- action on the library side: the catalog purge keeps an item a channel plans to air, and deleting
-- a collection a channel names is refused ("take it off the channel first").
CREATE TABLE channel_planned_items (
    channel_id      TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT NOT NULL REFERENCES library_items(id),
    PRIMARY KEY (channel_id, library_item_id)
);
CREATE INDEX idx_channel_planned_items_item ON channel_planned_items (library_item_id);
INSERT INTO channel_planned_items (channel_id, library_item_id)
SELECT DISTINCT c.id, json_extract(d.value, '$.LibraryItemID')
FROM channels c, json_each(c.desired_json) d
WHERE json_extract(d.value, '$.LibraryItemID') IN (SELECT id FROM library_items);

-- A collection id that names no collection is taken out of the policy, as it already matched no
-- title; left in, it would refuse every save of its channel.
UPDATE channels SET policy_json = json_set(policy_json, '$.scope.collections', json((
    SELECT json_group_array(x.value ORDER BY x.key) FROM json_each(channels.policy_json, '$.scope.collections') x
    WHERE x.value IN (SELECT id FROM library_collections))))
WHERE json_type(policy_json, '$.scope.collections') = 'array';
UPDATE channels SET policy_json = json_set(policy_json, '$.rules', json((
    SELECT json_group_array(json(CASE WHEN json_type(r.value, '$.what.collections') = 'array'
        THEN json_set(r.value, '$.what.collections', json((
            SELECT json_group_array(x.value ORDER BY x.key) FROM json_each(r.value, '$.what.collections') x
            WHERE x.value IN (SELECT id FROM library_collections))))
        ELSE r.value END) ORDER BY r.key)
    FROM json_each(channels.policy_json, '$.rules') r)))
WHERE json_type(policy_json, '$.rules') = 'array';

CREATE TABLE channel_scope_collections (
    channel_id    TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    collection_id TEXT NOT NULL REFERENCES library_collections(id),
    PRIMARY KEY (channel_id, collection_id)
);
CREATE INDEX idx_channel_scope_collections_collection ON channel_scope_collections (collection_id);
INSERT INTO channel_scope_collections (channel_id, collection_id)
SELECT c.id, x.value FROM channels c, json_each(c.policy_json, '$.scope.collections') x
UNION
SELECT c.id, x.value FROM channels c, json_each(c.policy_json, '$.rules') r,
    json_each(r.value, '$.what.collections') x;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
