-- +goose Up
-- Schema to a 10 (#2793, item 2): a channel's lineup names its titles by a foreign key. A title
-- added to a lineup by hand used to have no titles row until something asked for it, so
-- channel_lineup.title_key (00205) could not reference titles. Such a title now has a row in a new
-- state, pending: nothing asks for it on its own, the store reads it as no record, and any real
-- write of the title replaces it.
--
-- SQLite cannot change a CHECK or add a foreign key in place, so titles (with its child
-- title_requesters) and channel_lineup are rebuilt, the 00192 pattern: children renamed first,
-- created after the parent, dropped first.

-- titles and its requesters --------------------------------------------------------------------
ALTER TABLE title_requesters RENAME TO title_requesters_old;
ALTER TABLE titles RENAME TO titles_old;
CREATE TABLE titles (
    key             TEXT    PRIMARY KEY,
    title_json      TEXT    NOT NULL CHECK (json_valid(title_json)),
    state           TEXT    NOT NULL
        CHECK (state IN ('pending', 'wanted', 'requested', 'downloading', 'available', 'unavailable')),
    library_id      TEXT    NOT NULL DEFAULT '',
    requested_at    BIGINT  NOT NULL DEFAULT 0,
    deadline        BIGINT  NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT    NOT NULL DEFAULT '',
    updated_at      BIGINT  NOT NULL DEFAULT 0,
    progress        REAL    NOT NULL DEFAULT 0,
    eta_text        TEXT    NOT NULL DEFAULT '',
    download_status TEXT    NOT NULL DEFAULT '',
    arrived_at      BIGINT  NOT NULL DEFAULT 0,
    episodes_have   INTEGER NOT NULL DEFAULT 0,
    episodes_wanted INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE title_requesters (
    title_key  TEXT NOT NULL REFERENCES titles(key) ON DELETE CASCADE,
    person_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (title_key, person_id)
);
INSERT INTO titles SELECT * FROM titles_old;
INSERT INTO title_requesters SELECT * FROM title_requesters_old;
DROP TABLE title_requesters_old;
DROP TABLE titles_old;
CREATE INDEX idx_titles_state_deadline ON titles (state, deadline);
CREATE INDEX idx_titles_arrived ON titles (arrived_at);
CREATE INDEX idx_titles_library_id ON titles (library_id);
CREATE INDEX idx_title_requesters_person ON title_requesters (person_id);

-- a pending row for each title a lineup names that no row records ------------------------------
INSERT INTO titles (key, title_json, state, updated_at)
SELECT l.title_key,
       json_object('MediaType', substr(l.title_key, 1, instr(l.title_key, ':') - 1),
                   'Name', COALESCE(json_extract(MIN(l.entry), '$.Title'), ''),
                   'Year', COALESCE(json_extract(MIN(l.entry), '$.Year'), 0)),
       'pending', CAST(strftime('%s', 'now') AS INTEGER)
FROM channel_lineup l
WHERE NOT EXISTS (SELECT 1 FROM titles t WHERE t.key = l.title_key)
GROUP BY l.title_key;

-- channel_lineup, its title now a foreign key ---------------------------------------------------
ALTER TABLE channel_lineup RENAME TO channel_lineup_old;
CREATE TABLE channel_lineup (
    channel_id TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    title_key  TEXT    NOT NULL REFERENCES titles(key),
    entry      TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(entry)),
    PRIMARY KEY (channel_id, position)
);
INSERT INTO channel_lineup SELECT * FROM channel_lineup_old;
DROP TABLE channel_lineup_old;
CREATE INDEX idx_channel_lineup_title ON channel_lineup (title_key);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
