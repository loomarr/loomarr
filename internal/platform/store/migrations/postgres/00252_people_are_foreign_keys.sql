-- +goose Up
-- Schema to a 10 (#2793, item 1): the last columns naming a member are foreign keys to users.
--
--   - settings.updated_by, the admin who last changed a setting: cleared when they are deleted
--     (SET NULL). A system write had NULL, and a few rows held 'system' or ''; both become NULL;
--   - invitations.redeemed_by, the member an invitation made: cleared when they are deleted (SET
--     NULL), so a redeemed invitation no longer has to name its member;
--   - notification_destinations.owner_id, the member a personal destination belongs to: it goes
--     with them (CASCADE). An installation destination's '' becomes NULL;
--   - filler_pulls.decided_by, the admin who approved or dismissed a pull: cleared when they are
--     deleted (SET NULL); '' while pending becomes NULL.
--
-- An id naming no user becomes NULL, and a personal destination of no member is dropped, as
-- deleting that member now does. The CHECKs that tested these columns against '' test NULL; they
-- were declared unnamed, so each is found by its definition (the 00192 pattern).

-- +goose StatementBegin
DO $$
DECLARE r record;
BEGIN
    FOR r IN SELECT conrelid::regclass AS tbl, conname FROM pg_constraint
              WHERE contype = 'c'
                AND ((conrelid = 'invitations'::regclass AND pg_get_constraintdef(oid) LIKE '%redeemed_by%')
                  OR (conrelid = 'notification_destinations'::regclass AND pg_get_constraintdef(oid) LIKE '%owner_id%'))
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
    END LOOP;
END $$;
-- +goose StatementEnd

-- settings -------------------------------------------------------------------------------------
UPDATE settings s SET updated_by = (SELECT u.id FROM users u WHERE u.id = s.updated_by);
ALTER TABLE settings ADD CONSTRAINT settings_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_settings_updated_by ON settings (updated_by);

-- filler_pulls ---------------------------------------------------------------------------------
ALTER TABLE filler_pulls ALTER COLUMN decided_by DROP NOT NULL, ALTER COLUMN decided_by DROP DEFAULT;
UPDATE filler_pulls p SET decided_by = (SELECT u.id FROM users u WHERE u.id = p.decided_by);
ALTER TABLE filler_pulls ADD CONSTRAINT filler_pulls_decided_by_fkey
    FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_filler_pulls_decided_by ON filler_pulls (decided_by);

-- invitations ----------------------------------------------------------------------------------
ALTER TABLE invitations ALTER COLUMN redeemed_by DROP NOT NULL, ALTER COLUMN redeemed_by DROP DEFAULT;
UPDATE invitations i SET redeemed_by = (SELECT u.id FROM users u WHERE u.id = i.redeemed_by);
ALTER TABLE invitations
    ADD CONSTRAINT invitations_redeemed_by_fkey
        FOREIGN KEY (redeemed_by) REFERENCES users(id) ON DELETE SET NULL,
    ADD CONSTRAINT invitations_terminal_check CHECK (
        (status = 'pending' AND terminal_at = 0 AND redeemed_by IS NULL) OR
        (status = 'redeemed' AND terminal_at > 0) OR
        (status = 'expired' AND terminal_at = expires_at AND redeemed_by IS NULL) OR
        (status = 'revoked' AND terminal_at > 0 AND redeemed_by IS NULL)
    );
CREATE INDEX idx_invitations_redeemed_by ON invitations (redeemed_by);

-- notification_destinations --------------------------------------------------------------------
DELETE FROM notification_destinations d
WHERE d.scope = 'person' AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = d.owner_id);
ALTER TABLE notification_destinations ALTER COLUMN owner_id DROP NOT NULL, ALTER COLUMN owner_id DROP DEFAULT;
UPDATE notification_destinations SET owner_id = NULL WHERE owner_id = '';
ALTER TABLE notification_destinations
    ADD CONSTRAINT notification_destinations_owner_id_fkey
        FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT notification_destinations_owner_check CHECK (
        (scope = 'installation' AND owner_id IS NULL AND audience IN ('approvers', 'operators')) OR
        (scope = 'person' AND owner_id IS NOT NULL AND audience IN ('person', 'approvers', 'operators'))
    );
CREATE INDEX idx_notification_destinations_owner ON notification_destinations (owner_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
