-- +goose Up
-- Device pairing for native clients (§11, Shield P1). See the SQLite copy for the full rationale.
--
-- A TV has no keyboard, so it cannot use the password login that issues a session cookie, and the
-- only other credential Loomarr had was the household API_TOKEN — an ADMIN break-glass secret with
-- no user identity. A native client gets its own credential class instead.

CREATE TABLE device_pairings (
    device_code TEXT PRIMARY KEY,
    user_code   TEXT NOT NULL UNIQUE,
    user_id     TEXT REFERENCES users(id) ON DELETE CASCADE,
    device_name TEXT NOT NULL DEFAULT '',
    created_at  BIGINT NOT NULL DEFAULT 0,
    expires_at  BIGINT NOT NULL DEFAULT 0,
    approved_at BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_device_pairings_user_code ON device_pairings (user_code);
CREATE INDEX idx_device_pairings_expires ON device_pairings (expires_at);

-- Mirrors `sessions`: a revocable row, hash-at-rest. No expires_at — a TV idle for a month must not
-- log itself out; the credential is revoked explicitly instead.
CREATE TABLE device_tokens (
    token_hash   TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name  TEXT NOT NULL DEFAULT '',
    created_at   BIGINT NOT NULL DEFAULT 0,
    last_seen_at BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_device_tokens_user ON device_tokens (user_id);

-- +goose Down
DROP TABLE device_tokens;
DROP TABLE device_pairings;
