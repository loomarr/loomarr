-- +goose Up
-- PostgreSQL mirror of Invitation/grant persistence and owner-generic contact addresses (§11).
CREATE TABLE invitations (
    id                TEXT PRIMARY KEY,
    kind              TEXT NOT NULL CHECK (kind IN ('local', 'library')),
    username          TEXT NOT NULL DEFAULT '',
    library_user_id   TEXT NOT NULL DEFAULT '',
    display_name      TEXT NOT NULL DEFAULT '',
    identity_key      TEXT NOT NULL,
    role              TEXT NOT NULL CHECK (role IN ('member', 'admin')),
    status            TEXT NOT NULL CHECK (status IN ('pending', 'redeemed', 'expired', 'revoked')),
    created_at        BIGINT NOT NULL,
    expires_at        BIGINT NOT NULL,
    terminal_at       BIGINT NOT NULL DEFAULT 0,
    redeemed_by       TEXT NOT NULL DEFAULT '',
    CHECK (expires_at = created_at + 604800),
    CHECK (
        (kind = 'local' AND username <> '' AND library_user_id = '' AND display_name = '' AND identity_key = lower(btrim(username))) OR
        (kind = 'library' AND username = '' AND library_user_id <> '' AND identity_key = library_user_id)
    ),
    CHECK (
        (status = 'pending' AND terminal_at = 0 AND redeemed_by = '') OR
        (status = 'redeemed' AND terminal_at > 0 AND redeemed_by <> '') OR
        (status = 'expired' AND terminal_at = expires_at AND redeemed_by = '') OR
        (status = 'revoked' AND terminal_at > 0 AND redeemed_by = '')
    )
);

CREATE UNIQUE INDEX idx_invitations_pending_identity
    ON invitations (kind, identity_key) WHERE status = 'pending';
CREATE INDEX idx_invitations_terminal
    ON invitations (terminal_at) WHERE terminal_at > 0;

CREATE TABLE invitation_grants (
    token_hash       TEXT PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    invitation_id   TEXT NOT NULL REFERENCES invitations(id) ON DELETE CASCADE,
    grant_kind      TEXT NOT NULL CHECK (grant_kind = 'activation'),
    conveyance      TEXT NOT NULL CHECK (conveyance IN ('email', 'copy', 'qr')),
    created_at      BIGINT NOT NULL,
    expires_at      BIGINT NOT NULL,
    consumed_at     BIGINT NOT NULL DEFAULT 0,
    revoked_at      BIGINT NOT NULL DEFAULT 0,
    CHECK (expires_at > created_at AND expires_at <= created_at + 604800),
    CHECK (consumed_at = 0 OR revoked_at = 0)
);

CREATE INDEX idx_invitation_grants_invitation
    ON invitation_grants (invitation_id, created_at);

CREATE TABLE contact_addresses (
    owner_kind    TEXT NOT NULL CHECK (owner_kind IN ('user', 'invitation')),
    owner_id      TEXT NOT NULL,
    email         TEXT NOT NULL,
    normalized    TEXT NOT NULL UNIQUE,
    status        TEXT NOT NULL CHECK (status IN ('pending', 'verified')),
    provenance    TEXT NOT NULL CHECK (provenance IN ('admin', 'invitation', 'self')),
    created_at    BIGINT NOT NULL,
    verified_at   BIGINT,
    PRIMARY KEY (owner_kind, owner_id, status)
);

INSERT INTO contact_addresses
    (owner_kind, owner_id, email, normalized, status, provenance, created_at, verified_at)
SELECT 'user', user_id, email, normalized, status, provenance, created_at, verified_at
FROM user_contact_addresses;

DROP TABLE user_contact_addresses;

CREATE INDEX idx_contact_normalized_status
    ON contact_addresses (normalized, status);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
