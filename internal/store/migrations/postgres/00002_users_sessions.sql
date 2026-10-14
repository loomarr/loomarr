-- +goose Up
-- Postgres mirror of the users + sessions schema (§11). See the sqlite variant
-- for commentary; kept ANSI-compatible. Epoch BIGINT timestamps (§3).

CREATE TABLE users (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'member',
    disabled     BOOLEAN NOT NULL DEFAULT FALSE,
    quota        INTEGER NOT NULL DEFAULT 0,
    auto_approve BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   BIGINT NOT NULL DEFAULT 0,
    updated_at   BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at BIGINT NOT NULL DEFAULT 0,
    expires_at BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_sessions_user ON sessions (user_id);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
