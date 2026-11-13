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

-- titles ---------------------------------------------------------------------------------------
ALTER TABLE titles ALTER COLUMN library_id DROP NOT NULL, ALTER COLUMN library_id DROP DEFAULT;
UPDATE titles t SET library_id = NULL
WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = t.library_id);
ALTER TABLE titles ADD CONSTRAINT titles_library_id_fkey
    FOREIGN KEY (library_id) REFERENCES library_items(id);

-- airings and scheduled_airings ----------------------------------------------------------------
DELETE FROM airings a WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = a.library_item_id);
DELETE FROM scheduled_airings a WHERE NOT EXISTS (SELECT 1 FROM library_items i WHERE i.id = a.library_item_id);
ALTER TABLE airings ADD CONSTRAINT airings_library_item_id_fkey
    FOREIGN KEY (library_item_id) REFERENCES library_items(id);
ALTER TABLE scheduled_airings ADD CONSTRAINT scheduled_airings_library_item_id_fkey
    FOREIGN KEY (library_item_id) REFERENCES library_items(id);
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
SELECT DISTINCT c.id, d.value ->> 'LibraryItemID'
FROM channels c, jsonb_array_elements(c.desired_json) d
WHERE d.value ->> 'LibraryItemID' IN (SELECT id FROM library_items);

-- A collection id that names no collection is taken out of the policy, as it already matched no
-- title; left in, it would refuse every save of its channel.
UPDATE channels SET policy_json = jsonb_set(policy_json, '{scope,collections}', COALESCE((
    SELECT jsonb_agg(x.value ORDER BY x.ord) FROM jsonb_array_elements(policy_json #> '{scope,collections}')
        WITH ORDINALITY AS x(value, ord)
    WHERE x.value #>> '{}' IN (SELECT id FROM library_collections)), '[]'::jsonb))
WHERE jsonb_typeof(policy_json #> '{scope,collections}') = 'array';
UPDATE channels SET policy_json = jsonb_set(policy_json, '{rules}', (
    SELECT jsonb_agg(CASE WHEN jsonb_typeof(r.value #> '{what,collections}') = 'array'
        THEN jsonb_set(r.value, '{what,collections}', COALESCE((
            SELECT jsonb_agg(x.value ORDER BY x.ord) FROM jsonb_array_elements(r.value #> '{what,collections}')
                WITH ORDINALITY AS x(value, ord)
            WHERE x.value #>> '{}' IN (SELECT id FROM library_collections)), '[]'::jsonb))
        ELSE r.value END ORDER BY r.ord)
    FROM jsonb_array_elements(policy_json -> 'rules') WITH ORDINALITY AS r(value, ord)))
WHERE jsonb_typeof(policy_json -> 'rules') = 'array' AND jsonb_array_length(policy_json -> 'rules') > 0;

CREATE TABLE channel_scope_collections (
    channel_id    TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    collection_id TEXT NOT NULL REFERENCES library_collections(id),
    PRIMARY KEY (channel_id, collection_id)
);
CREATE INDEX idx_channel_scope_collections_collection ON channel_scope_collections (collection_id);
INSERT INTO channel_scope_collections (channel_id, collection_id)
SELECT c.id, x.value FROM channels c, jsonb_array_elements_text(c.policy_json #> '{scope,collections}') x(value)
WHERE jsonb_typeof(c.policy_json #> '{scope,collections}') = 'array'
UNION
SELECT c.id, x.value FROM channels c, jsonb_array_elements(c.policy_json -> 'rules') r(value),
    jsonb_array_elements_text(r.value #> '{what,collections}') x(value)
WHERE jsonb_typeof(c.policy_json -> 'rules') = 'array' AND jsonb_typeof(r.value #> '{what,collections}') = 'array';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
