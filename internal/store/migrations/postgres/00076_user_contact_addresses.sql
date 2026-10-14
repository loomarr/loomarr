-- +goose Up
-- PostgreSQL mirror of the optional person contact-address lifecycle (§11).
CREATE TABLE user_contact_addresses (
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email         TEXT NOT NULL,
    normalized    TEXT NOT NULL UNIQUE,
    status        TEXT NOT NULL CHECK (status IN ('pending', 'verified')),
    provenance    TEXT NOT NULL CHECK (provenance IN ('admin', 'invitation', 'self')),
    created_at    BIGINT NOT NULL,
    verified_at   BIGINT,
    PRIMARY KEY (user_id, status)
);

CREATE INDEX idx_user_contact_normalized_status
    ON user_contact_addresses (normalized, status);

-- Existing users stay contactless. Provider/SSO metadata cannot silently become trusted contact.

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
