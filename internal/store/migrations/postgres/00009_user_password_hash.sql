-- +goose Up
-- Local-credential support (§11 identity rework): a nullable password_hash on
-- users. Set ⇒ a LOCAL user (bcrypt-verified in-app); NULL ⇒ an IMPORTED
-- media-server user (verified against Emby/Jellyfin). The null-vs-set state is
-- the credential-path discriminator — no separate column needed.
--
-- The `id` primary key widens in MEANING (not shape): still the media-server user
-- id for imported users, now also a Loomarr-minted id for local users. Existing
-- rows are all imported/media-server users → NULL password_hash, unchanged.
--
-- Forward-only (§16). Third real ALTER (after 00007 policy_json, 00008 settings
-- audit). users holds durable rows, so ALTER in place, never drop.

ALTER TABLE users ADD COLUMN password_hash TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN password_hash;
