-- +goose Up
-- Provisioning records (§3–§4) and the settings KV (§5). Timestamps are Unix
-- epoch BIGINT, not native datetime, so the schema stays dialect-neutral across
-- SQLite and Postgres (§3 note). The title blob is JSON (identity is `key`).

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

-- ClaimDueTitles filters in-flight rows by deadline; index the hot path.
CREATE INDEX idx_titles_state_deadline ON titles (state, deadline);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE settings;
DROP TABLE titles;
