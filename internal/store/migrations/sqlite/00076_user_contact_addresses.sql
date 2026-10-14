-- +goose Up
-- Optional person contact data (§11). The compound primary key structurally permits at most one
-- verified address and one pending replacement per person. Global normalized uniqueness keeps
-- public recovery unambiguous across both states.
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
