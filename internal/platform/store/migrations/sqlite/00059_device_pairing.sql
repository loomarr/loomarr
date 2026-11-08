-- +goose Up
-- Device pairing for native clients (§11, Shield P1).
--
-- A TV has no keyboard, so it cannot use the password login that issues a session cookie, and the
-- only other credential Loomarr had was the household API_TOKEN — an ADMIN break-glass secret with
-- no user identity. Putting that on a shared living-room device is a privilege escalation, so a
-- native client gets its own credential class instead.
--
-- Two tables because a pairing has two distinct lifetimes: a short-lived code a human types, and a
-- long-lived token the device keeps.

-- The short-lived half. A device POSTs for a code, shows it on screen, and polls; a signed-in human
-- approves it in the web UI. Rows are consumed on approval and swept after expiry.
CREATE TABLE device_pairings (
    device_code TEXT PRIMARY KEY,          -- SHA-256 of the device's polling secret, never plaintext
    user_code   TEXT NOT NULL UNIQUE,      -- the short human-typed code, shown on the TV
    user_id     TEXT,                      -- set at approval; NULL while pending
    device_name TEXT NOT NULL DEFAULT '',  -- client-supplied label ("Living Room Shield")
    created_at  BIGINT NOT NULL DEFAULT 0,
    expires_at  BIGINT NOT NULL DEFAULT 0, -- the pairing window closes fast; a stale code must not linger
    approved_at BIGINT NOT NULL DEFAULT 0, -- 0 = still pending
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_device_pairings_user_code ON device_pairings (user_code);
CREATE INDEX idx_device_pairings_expires ON device_pairings (expires_at);

-- The long-lived half. Mirrors `sessions` deliberately: a revocable store row rather than a JWT, and
-- only the token's SHA-256 is kept, so a database read never yields a usable credential.
--
-- ⚠ NO expires_at, unlike sessions. A TV that sits unused for a month must not silently log itself
-- out — the credential is revoked explicitly instead, which is exactly what per-device rows buy.
CREATE TABLE device_tokens (
    token_hash   TEXT PRIMARY KEY,          -- SHA-256 of the device token (§11)
    user_id      TEXT NOT NULL,             -- the member who approved it; the device inherits ITS role
    device_name  TEXT NOT NULL DEFAULT '',
    created_at   BIGINT NOT NULL DEFAULT 0,
    last_seen_at BIGINT NOT NULL DEFAULT 0, -- so the revocation UI can show what is actually in use
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_device_tokens_user ON device_tokens (user_id);

-- +goose Down
DROP TABLE device_tokens;
DROP TABLE device_pairings;
