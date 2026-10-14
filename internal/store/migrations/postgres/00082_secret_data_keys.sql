-- +goose Up
-- Raw data-encryption keys never enter this table. wrapped_key is an authenticated
-- AES-GCM envelope protected by the installation key held outside the database (§15).
CREATE TABLE secret_data_keys (
    id          TEXT PRIMARY KEY,
    wrapped_key TEXT NOT NULL,
    active      INTEGER NOT NULL CHECK (active IN (0, 1)),
    created_at  BIGINT NOT NULL
);

CREATE UNIQUE INDEX idx_secret_data_keys_one_active
    ON secret_data_keys (active) WHERE active = 1;

-- The fingerprint is safe to retain with the database. It detects a wrong or
-- missing external installation key before Loomarr attempts to unwrap a DEK.
CREATE TABLE secret_protection_metadata (
    singleton                   INTEGER PRIMARY KEY CHECK (singleton = 1),
    installation_key_fingerprint TEXT NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
