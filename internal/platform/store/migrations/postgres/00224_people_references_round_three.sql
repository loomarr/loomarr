-- +goose Up
-- Schema to a 10 (#2793, item 1): the last three columns that name a person reference users by
-- foreign key.
--
--   - notification_destinations.owner_id, the person a person-scoped destination belongs to, goes
--     with that person (CASCADE): the destination is theirs, such as their own browser;
--   - invitations.redeemed_by, the account an invitation became, is forgotten when that account is
--     deleted (SET NULL), and the invitation stays as history;
--   - filler_pulls.decided_by, the admin who approved or dismissed a pull, is forgotten when that
--     admin is deleted (SET NULL), and the decision stays.
--
-- Each used '' for "no one", which no user has as its id, so it becomes NULL, as does a name of a
-- person who is gone. The CHECKs that tied owner_id and redeemed_by to the row's scope or status
-- now ask IS NULL or IS NOT NULL, except that a redeemed invitation may forget its account.

-- filler_pulls.decided_by -----------------------------------------------------------------------
ALTER TABLE filler_pulls ALTER COLUMN decided_by DROP NOT NULL, ALTER COLUMN decided_by DROP DEFAULT;
UPDATE filler_pulls p SET decided_by = NULL
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = p.decided_by);
ALTER TABLE filler_pulls ADD CONSTRAINT filler_pulls_decided_by_fkey
    FOREIGN KEY (decided_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_filler_pulls_decided_by ON filler_pulls (decided_by);

-- notification_destinations.owner_id ------------------------------------------------------------
-- A person-scoped destination whose person is gone goes, as deleting the person now does.
DELETE FROM notification_destinations d
WHERE d.owner_id <> '' AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = d.owner_id);
ALTER TABLE notification_destinations DROP CONSTRAINT notification_destinations_check;
ALTER TABLE notification_destinations
    ALTER COLUMN owner_id DROP NOT NULL,
    ALTER COLUMN owner_id DROP DEFAULT;
UPDATE notification_destinations SET owner_id = NULL WHERE owner_id = '';
ALTER TABLE notification_destinations ADD CONSTRAINT notification_destinations_check CHECK (
    (scope = 'installation' AND owner_id IS NULL AND audience IN ('approvers', 'operators')) OR
    (scope = 'person' AND owner_id IS NOT NULL AND audience IN ('person', 'approvers', 'operators'))
);
ALTER TABLE notification_destinations ADD CONSTRAINT notification_destinations_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX idx_notification_destinations_owner ON notification_destinations (owner_id);

-- invitations.redeemed_by -----------------------------------------------------------------------
ALTER TABLE invitations DROP CONSTRAINT invitations_check2;
ALTER TABLE invitations ALTER COLUMN redeemed_by DROP NOT NULL, ALTER COLUMN redeemed_by DROP DEFAULT;
UPDATE invitations i SET redeemed_by = NULL
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = i.redeemed_by);
ALTER TABLE invitations ADD CONSTRAINT invitations_check2 CHECK (
    (status = 'pending' AND terminal_at = 0 AND redeemed_by IS NULL) OR
    (status = 'redeemed' AND terminal_at > 0) OR
    (status = 'expired' AND terminal_at = expires_at AND redeemed_by IS NULL) OR
    (status = 'revoked' AND terminal_at > 0 AND redeemed_by IS NULL)
);
ALTER TABLE invitations ADD CONSTRAINT invitations_redeemed_by_fkey
    FOREIGN KEY (redeemed_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_invitations_redeemed_by ON invitations (redeemed_by);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
