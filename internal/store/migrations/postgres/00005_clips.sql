-- +goose Up
-- Postgres mirror of the clips schema (§10). See the sqlite variant for
-- commentary; kept ANSI-compatible. ai_tagged is INTEGER (0/1) to match the
-- SQLite dialect and the shared scan code (§5 one-suite-two-backends).

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

-- +goose Down
DROP TABLE clips;
