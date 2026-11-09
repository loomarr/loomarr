-- +goose Up
-- Schema to a 10 (#2793, item 2): a channel's lineup names its titles by a foreign key. A title
-- added to a lineup by hand used to have no titles row until something asked for it, so
-- channel_lineup.title_key (00205) could not reference titles. Such a title now has a row in a new
-- state, pending: nothing asks for it on its own, the store reads it as no record, and any real
-- write of the title replaces it.

-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
    FOR c IN SELECT conname FROM pg_constraint
              WHERE conrelid = 'titles'::regclass AND contype = 'c'
                AND pg_get_constraintdef(oid) LIKE '%state%'
    LOOP
        EXECUTE format('ALTER TABLE titles DROP CONSTRAINT %I', c);
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE titles ADD CONSTRAINT titles_state_check
    CHECK (state IN ('pending', 'wanted', 'requested', 'downloading', 'available', 'unavailable'));

-- a pending row for each title a lineup names that no row records
INSERT INTO titles (key, title_json, state, updated_at)
SELECT l.title_key,
       jsonb_build_object('MediaType', split_part(l.title_key, ':', 1),
                          'Name', COALESCE(MIN(l.entry ->> 'Title'), ''),
                          'Year', COALESCE(MIN((l.entry ->> 'Year')::bigint), 0)),
       'pending', EXTRACT(EPOCH FROM now())::bigint
FROM channel_lineup l
WHERE NOT EXISTS (SELECT 1 FROM titles t WHERE t.key = l.title_key)
GROUP BY l.title_key;

ALTER TABLE channel_lineup ADD CONSTRAINT channel_lineup_title_key_fkey
    FOREIGN KEY (title_key) REFERENCES titles(key);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
