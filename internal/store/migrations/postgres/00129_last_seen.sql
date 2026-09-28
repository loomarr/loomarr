-- +goose Up
-- People's "Last seen" and "Where you're signed in" (#1667).
--
-- A session records when it was last used and a coarse client label ("Firefox on macOS", family
-- and system only), both written by the sliding-expiry touch every authenticated request already
-- makes. users.last_seen_at follows the person across their browser sessions and their paired
-- TVs, and outlives purged sessions. No backfill: nothing recorded use before this.
ALTER TABLE sessions ADD COLUMN last_seen_at BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN client_label TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN last_seen_at BIGINT NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
