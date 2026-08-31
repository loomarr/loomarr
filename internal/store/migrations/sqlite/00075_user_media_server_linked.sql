-- +goose Up
-- Provider linkage is independent from possession of an offline password verifier.
ALTER TABLE users ADD COLUMN media_server_linked BOOLEAN NOT NULL DEFAULT false;

-- Under the old schema, NULL password_hash was the credential-path discriminator
-- for an imported Emby/Jellyfin user. Preserve that meaning during upgrade.
UPDATE users SET media_server_linked = true WHERE password_hash IS NULL;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
