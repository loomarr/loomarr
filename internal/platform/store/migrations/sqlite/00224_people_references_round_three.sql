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
--
-- filler_pulls.decided_by has no CHECK, so it is moved in place (the 00211 pattern). The other two
-- tables carry those CHECKs, which SQLite can't change in place, so they are rebuilt. Renaming a
-- parent repoints its children's foreign keys at the renamed copy, and dropping that copy would
-- cascade-delete them, so invitation_grants is rebuilt with invitations (the 00188 pattern): it is
-- renamed first, created after its parent, and dropped first.

-- filler_pulls.decided_by -----------------------------------------------------------------------
ALTER TABLE filler_pulls ADD COLUMN decided_by_ref TEXT REFERENCES users(id) ON DELETE SET NULL;
UPDATE filler_pulls SET decided_by_ref = decided_by
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = filler_pulls.decided_by);
ALTER TABLE filler_pulls DROP COLUMN decided_by;
ALTER TABLE filler_pulls RENAME COLUMN decided_by_ref TO decided_by;
CREATE INDEX idx_filler_pulls_decided_by ON filler_pulls (decided_by);

-- notification_destinations.owner_id ------------------------------------------------------------
ALTER TABLE notification_destinations RENAME TO notification_destinations_old;
CREATE TABLE notification_destinations (
    id                       TEXT    PRIMARY KEY,
    means                    TEXT    NOT NULL CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    )),
    label                    TEXT    NOT NULL,
    scope                    TEXT    NOT NULL CHECK (scope IN ('installation', 'person')),
    owner_id                 TEXT    REFERENCES users(id) ON DELETE CASCADE,
    audience                 TEXT    NOT NULL CHECK (audience IN ('person', 'approvers', 'operators')),
    topics_json              TEXT    NOT NULL CHECK (json_valid(topics_json)),
    enabled                  BOOLEAN NOT NULL CHECK (enabled IN (0, 1)),
    configuration_json       TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(configuration_json)),
    credentials_encrypted    TEXT    NOT NULL,
    created_at               BIGINT  NOT NULL,
    updated_at               BIGINT  NOT NULL,
    credential_keys_json     TEXT    CHECK (json_valid(credential_keys_json)),
    subscription_fingerprint TEXT    NOT NULL DEFAULT '',
    CHECK (
        (scope = 'installation' AND owner_id IS NULL AND audience IN ('approvers', 'operators')) OR
        (scope = 'person' AND owner_id IS NOT NULL AND audience IN ('person', 'approvers', 'operators'))
    )
);
-- A person-scoped destination whose person is gone goes, as deleting the person now does.
INSERT INTO notification_destinations
    (id, means, label, scope, owner_id, audience, topics_json, enabled, configuration_json,
     credentials_encrypted, created_at, updated_at, credential_keys_json, subscription_fingerprint)
SELECT id, means, label, scope, NULLIF(owner_id, ''), audience, topics_json, enabled, configuration_json,
       credentials_encrypted, created_at, updated_at, credential_keys_json, subscription_fingerprint
FROM notification_destinations_old d
WHERE d.owner_id = '' OR EXISTS (SELECT 1 FROM users u WHERE u.id = d.owner_id);
DROP TABLE notification_destinations_old;
CREATE INDEX idx_notification_destinations_routing
    ON notification_destinations (enabled, scope, audience, means, id);
CREATE UNIQUE INDEX idx_notification_destinations_web_push_subscription
    ON notification_destinations (owner_id, subscription_fingerprint)
    WHERE means = 'web_push' AND subscription_fingerprint <> '';
CREATE INDEX idx_notification_destinations_owner ON notification_destinations (owner_id);

-- invitations.redeemed_by, with invitation_grants -----------------------------------------------
ALTER TABLE invitation_grants RENAME TO invitation_grants_old;
ALTER TABLE invitations RENAME TO invitations_old;
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
    redeemed_by       TEXT REFERENCES users(id) ON DELETE SET NULL,
    CHECK (expires_at = created_at + 604800),
    CHECK (
        (kind = 'local' AND username <> '' AND library_user_id = '' AND display_name = '' AND identity_key = lower(trim(username))) OR
        (kind = 'library' AND username = '' AND library_user_id <> '' AND identity_key = library_user_id)
    ),
    CHECK (
        (status = 'pending' AND terminal_at = 0 AND redeemed_by IS NULL) OR
        (status = 'redeemed' AND terminal_at > 0) OR
        (status = 'expired' AND terminal_at = expires_at AND redeemed_by IS NULL) OR
        (status = 'revoked' AND terminal_at > 0 AND redeemed_by IS NULL)
    )
);
INSERT INTO invitations
    (id, kind, username, library_user_id, display_name, identity_key, role, status, created_at,
     expires_at, terminal_at, redeemed_by)
SELECT id, kind, username, library_user_id, display_name, identity_key, role, status, created_at,
       expires_at, terminal_at,
       CASE WHEN EXISTS (SELECT 1 FROM users u WHERE u.id = i.redeemed_by) THEN i.redeemed_by END
FROM invitations_old i;
CREATE TABLE invitation_grants (
    token_hash       TEXT PRIMARY KEY CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
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
INSERT INTO invitation_grants SELECT * FROM invitation_grants_old;
DROP TABLE invitation_grants_old;
DROP TABLE invitations_old;
CREATE UNIQUE INDEX idx_invitations_pending_identity
    ON invitations (kind, identity_key) WHERE status = 'pending';
CREATE INDEX idx_invitations_terminal
    ON invitations (terminal_at) WHERE terminal_at > 0;
CREATE INDEX idx_invitations_redeemed_by ON invitations (redeemed_by);
CREATE INDEX idx_invitation_grants_invitation
    ON invitation_grants (invitation_id, created_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
