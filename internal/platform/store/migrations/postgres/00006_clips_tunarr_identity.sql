-- +goose Up
-- Postgres mirror of the §10 filler-redesign clip-identity change. See the sqlite
-- variant for commentary. Clip identity moves media-server-item-id →
-- Tunarr-`local`-source-program-uuid; forward-only (§16); drop + recreate EMPTY
-- (identity source changed → old rows invalid; next sync repopulates from Tunarr).

DROP TABLE clips;

CREATE TABLE clips (
    tunarr_program_id TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    kind              TEXT NOT NULL DEFAULT 'interstitial',
    era               INTEGER NOT NULL DEFAULT 0,
    audience          TEXT NOT NULL DEFAULT '',
    category          TEXT NOT NULL DEFAULT '',
    duration_ms       BIGINT NOT NULL DEFAULT 0,
    rating            TEXT NOT NULL DEFAULT '',
    source            TEXT NOT NULL DEFAULT '',
    ai_tagged         INTEGER NOT NULL DEFAULT 0,
    updated_at        BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);

-- +goose Down
DROP TABLE clips;

CREATE TABLE clips (
    library_item_id TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'interstitial',
    era             INTEGER NOT NULL DEFAULT 0,
    audience        TEXT NOT NULL DEFAULT '',
    category        TEXT NOT NULL DEFAULT '',
    duration_ms     BIGINT NOT NULL DEFAULT 0,
    rating          TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT '',
    ai_tagged       INTEGER NOT NULL DEFAULT 0,
    updated_at      BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_clips_kind ON clips (kind);
CREATE INDEX idx_clips_match ON clips (audience, era);
