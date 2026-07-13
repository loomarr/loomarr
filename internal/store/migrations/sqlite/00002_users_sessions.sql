-- +goose Up
-- Users imported/synced from the media server (§11), keyed by media-server user
-- id. Sessions are revocable rows (not JWTs) with the token SHA-256-hashed at
-- rest — a DB read never yields a usable cookie. Epoch BIGINT timestamps (§3).

CREATE TABLE users (
    id         TEXT PRIMARY KEY,          -- media-server user id (§11)
    name       TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'member', -- admin | member
    disabled   INTEGER NOT NULL DEFAULT 0,
    quota      INTEGER NOT NULL DEFAULT 0,  -- pending-acquisition cap; 0 = default
    auto_approve INTEGER NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL DEFAULT 0,
    updated_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,          -- SHA-256 of the cookie token (§11)
    user_id    TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT 0,
    expires_at BIGINT NOT NULL DEFAULT 0, -- sliding TTL; janitor purges past this
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_sessions_user ON sessions (user_id);
CREATE INDEX idx_sessions_expires ON sessions (expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
