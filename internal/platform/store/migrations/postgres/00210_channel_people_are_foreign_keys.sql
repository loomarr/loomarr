-- +goose Up
-- Schema to a 10 (#2793, item 1): who asked for a channel, and which channel feedback is about,
-- are foreign keys.
--
--   - channels.requested_by references users; a deleted person leaves the channel with no
--     requester (SET NULL), as one no member asked for;
--   - discovery_feedback.scope_id references channels and goes with its channel, which
--     DeleteChannel used to do by hand.
--
-- requested_by and scope_id used '' for "none", which no row has as its id, so they become
-- nullable and each '' turns into NULL; the feedback scope CHECK tests scope_id for NULL. A
-- requester naming no user is cleared the same way, and feedback whose channel is gone is
-- dropped. Feedback's actor stays a plain id: the API token writes one that is no user.
ALTER TABLE channels ALTER COLUMN requested_by DROP NOT NULL, ALTER COLUMN requested_by DROP DEFAULT;
UPDATE channels c SET requested_by = NULL
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = c.requested_by);
ALTER TABLE channels ADD CONSTRAINT channels_requested_by_fkey
    FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_channels_requested_by ON channels (requested_by);

-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
    FOR c IN SELECT conname FROM pg_constraint
             WHERE conrelid = 'discovery_feedback'::regclass AND contype = 'c'
               AND pg_get_constraintdef(oid) LIKE '%scope_id%'
    LOOP
        EXECUTE format('ALTER TABLE discovery_feedback DROP CONSTRAINT %I', c);
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE discovery_feedback ALTER COLUMN scope_id DROP NOT NULL, ALTER COLUMN scope_id DROP DEFAULT;
UPDATE discovery_feedback SET scope_id = NULL WHERE scope_id = '';
DELETE FROM discovery_feedback f
WHERE f.scope = 'channel' AND NOT EXISTS (SELECT 1 FROM channels c WHERE c.id = f.scope_id);
ALTER TABLE discovery_feedback ADD CONSTRAINT discovery_feedback_scope_id_check
    CHECK ((scope = 'household' AND scope_id IS NULL) OR (scope = 'channel' AND scope_id IS NOT NULL));
ALTER TABLE discovery_feedback ADD CONSTRAINT discovery_feedback_scope_id_fkey
    FOREIGN KEY (scope_id) REFERENCES channels(id) ON DELETE CASCADE;
CREATE INDEX idx_discovery_feedback_scope_id ON discovery_feedback (scope_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
