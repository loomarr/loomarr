-- +goose Up
-- Postgres mirror of the SQLite init (§5). Kept ANSI-compatible; the only
-- dialect differences across the whole schema live in these per-dialect files
-- and in ClaimDueTitles. Epoch BIGINT timestamps, JSON title blob (§3 note).

CREATE TABLE titles (
    key          TEXT PRIMARY KEY,
    title_json   TEXT NOT NULL,
    state        TEXT NOT NULL,
    library_id   TEXT NOT NULL DEFAULT '',
    requested_at BIGINT NOT NULL DEFAULT 0,
    deadline     BIGINT NOT NULL DEFAULT 0,
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    updated_at   BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_titles_state_deadline ON titles (state, deadline);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE settings;
DROP TABLE titles;
