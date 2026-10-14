-- +goose Up
-- Local-password recovery lifecycle and hashed bearer grants (§11). Neither table may contain a
-- plaintext token, rendered URL, password, or email address.
CREATE TABLE password_recoveries (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      TEXT NOT NULL CHECK (status IN ('pending', 'redeemed', 'expired', 'revoked')),
    created_at  BIGINT NOT NULL,
    expires_at  BIGINT NOT NULL,
    terminal_at BIGINT NOT NULL DEFAULT 0,
    CHECK (expires_at = created_at + 1800),
    CHECK (
        (status = 'pending' AND terminal_at = 0) OR
        (status = 'expired' AND terminal_at = expires_at) OR
        (status IN ('redeemed', 'revoked') AND terminal_at > 0)
    )
);

CREATE UNIQUE INDEX idx_password_recoveries_pending_user
    ON password_recoveries (user_id) WHERE status = 'pending';
CREATE INDEX idx_password_recoveries_terminal
    ON password_recoveries (terminal_at) WHERE terminal_at > 0;

CREATE TABLE password_recovery_grants (
    token_hash   TEXT PRIMARY KEY CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
    recovery_id  TEXT NOT NULL REFERENCES password_recoveries(id) ON DELETE CASCADE,
    created_at   BIGINT NOT NULL,
    expires_at   BIGINT NOT NULL,
    consumed_at  BIGINT NOT NULL DEFAULT 0,
    revoked_at   BIGINT NOT NULL DEFAULT 0,
    CHECK (expires_at > created_at AND expires_at <= created_at + 1800),
    CHECK (consumed_at = 0 OR revoked_at = 0)
);

CREATE INDEX idx_password_recovery_grants_recovery
    ON password_recovery_grants (recovery_id, created_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
