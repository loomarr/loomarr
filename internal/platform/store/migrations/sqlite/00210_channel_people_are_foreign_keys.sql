-- +goose Up
-- Schema to a 10 (#2793, item 1): who asked for a channel, and who gave feedback about what, are
-- foreign keys.
--
--   - channels.requested_by references users; a deleted person leaves the channel with no
--     requester (SET NULL), as one no member asked for;
--   - discovery_feedback.actor_id references users and goes with its person, like their favourites;
--   - discovery_feedback.scope_id references channels and goes with its channel, which
--     DeleteChannel used to do by hand.
--
-- requested_by and scope_id used '' for "none", which no row has as its id, so they become
-- nullable and the copy turns each '' into NULL; the feedback scope CHECK tests scope_id for NULL.
-- A requester naming no user is cleared the same way, and feedback whose person or channel is gone
-- is dropped.
--
-- SQLite cannot add a foreign key in place, so channels is rebuilt with its children (the 00192
-- pattern): children are renamed first, created after the parent, and dropped first.

-- channels and its children ---------------------------------------------------------------------
ALTER TABLE airings RENAME TO airings_old;
ALTER TABLE scheduled_airings RENAME TO scheduled_airings_old;
ALTER TABLE user_favourite_channels RENAME TO user_favourite_channels_old;
ALTER TABLE user_recent_channels RENAME TO user_recent_channels_old;
ALTER TABLE channel_images RENAME TO channel_images_old;
ALTER TABLE channel_requesters RENAME TO channel_requesters_old;
ALTER TABLE channel_lineup RENAME TO channel_lineup_old;
ALTER TABLE channels RENAME TO channels_old;

CREATE TABLE channels (
    id                 TEXT    PRIMARY KEY,
    intent_ref         TEXT    NOT NULL DEFAULT '',
    name               TEXT    NOT NULL,
    number             INTEGER NOT NULL,
    grp                TEXT    NOT NULL DEFAULT '',   -- the channel's group ("group" is reserved)
    logo               TEXT    NOT NULL DEFAULT '',
    strategy           TEXT    NOT NULL,
    tunarr_id          TEXT    NOT NULL DEFAULT '',
    status             TEXT    NOT NULL
        CHECK (status IN ('building', 'live', 'drifted', 'empty', 'detached', 'paused')),
    shuffle_seed       BIGINT  NOT NULL DEFAULT 0,
    desired_json       TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(desired_json)),
    -- reconcile_deadline drives ClaimDueChannels: a channel is due when its deadline is at or
    -- before now; claiming leases it so two replicas don't reconcile one channel at once.
    reconcile_deadline BIGINT  NOT NULL DEFAULT 0,
    updated_at         BIGINT  NOT NULL DEFAULT 0,
    policy_json        TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(policy_json)),
    broadcast_codec    TEXT    NOT NULL DEFAULT 'h264',
    revision           BIGINT  NOT NULL DEFAULT 1 CHECK (revision >= 1),
    playout_anchor     BIGINT  NOT NULL DEFAULT 0 CHECK (playout_anchor >= 0),
    created_at         BIGINT  NOT NULL DEFAULT 0,
    intent_json        TEXT    CHECK (json_valid(intent_json)),
    requested_by       TEXT    REFERENCES users(id) ON DELETE SET NULL
);
CREATE TABLE airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
CREATE TABLE scheduled_airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,
    aired_at        BIGINT NOT NULL,
    recorded_at     BIGINT NOT NULL,
    key             TEXT   NOT NULL DEFAULT '',
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
CREATE TABLE user_favourite_channels (
    user_id     TEXT   NOT NULL,
    channel_id  TEXT   NOT NULL,
    added_at_ms BIGINT NOT NULL CHECK (added_at_ms > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
CREATE TABLE user_recent_channels (
    user_id     TEXT   NOT NULL,
    channel_id  TEXT   NOT NULL,
    tuned_at_ms BIGINT NOT NULL CHECK (tuned_at_ms > 0),
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
CREATE TABLE channel_images (
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    PRIMARY KEY (channel_id, role, image_hash)
);
CREATE TABLE channel_requesters (
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    person_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at BIGINT NOT NULL,
    PRIMARY KEY (channel_id, person_id)
);
CREATE TABLE channel_lineup (
    channel_id TEXT    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    title_key  TEXT    NOT NULL REFERENCES titles(key),
    entry      TEXT    NOT NULL DEFAULT '{}' CHECK (json_valid(entry)),
    PRIMARY KEY (channel_id, position)
);

INSERT INTO channels
SELECT id, intent_ref, name, number, grp, logo, strategy, tunarr_id, status, shuffle_seed, desired_json,
       reconcile_deadline, updated_at, policy_json, broadcast_codec, revision, playout_anchor, created_at,
       intent_json,
       CASE WHEN EXISTS (SELECT 1 FROM users u WHERE u.id = c.requested_by) THEN c.requested_by END
FROM channels_old c;
INSERT INTO airings SELECT * FROM airings_old;
INSERT INTO scheduled_airings SELECT * FROM scheduled_airings_old;
INSERT INTO user_favourite_channels SELECT * FROM user_favourite_channels_old;
INSERT INTO user_recent_channels SELECT * FROM user_recent_channels_old;
INSERT INTO channel_images SELECT * FROM channel_images_old;
INSERT INTO channel_requesters SELECT * FROM channel_requesters_old;
INSERT INTO channel_lineup SELECT * FROM channel_lineup_old;

DROP TABLE airings_old;
DROP TABLE scheduled_airings_old;
DROP TABLE user_favourite_channels_old;
DROP TABLE user_recent_channels_old;
DROP TABLE channel_images_old;
DROP TABLE channel_requesters_old;
DROP TABLE channel_lineup_old;
DROP TABLE channels_old;

CREATE UNIQUE INDEX idx_channels_intent_ref ON channels (intent_ref) WHERE intent_ref <> '';
CREATE UNIQUE INDEX idx_channels_number ON channels (number);
CREATE INDEX idx_channels_status_deadline ON channels (status, reconcile_deadline);
CREATE INDEX idx_channels_requested_by ON channels (requested_by);
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);
CREATE INDEX idx_airings_library_item ON airings (library_item_id);
CREATE INDEX idx_scheduled_airings_channel ON scheduled_airings (channel_id, recorded_at);
CREATE INDEX idx_user_favourite_channels_channel ON user_favourite_channels (channel_id);
CREATE INDEX idx_user_recent_channels_channel ON user_recent_channels (channel_id);
CREATE INDEX idx_user_recent_channels_user_tuned ON user_recent_channels (user_id, tuned_at_ms DESC);
CREATE INDEX idx_channel_images_image ON channel_images (image_hash);
CREATE INDEX idx_channel_requesters_person ON channel_requesters (person_id);
CREATE INDEX idx_channel_lineup_title ON channel_lineup (title_key);

-- discovery_feedback ------------------------------------------------------------------------------
ALTER TABLE discovery_feedback RENAME TO discovery_feedback_old;
CREATE TABLE discovery_feedback (
    id            TEXT   PRIMARY KEY,
    actor_id      TEXT   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope         TEXT   NOT NULL CHECK (scope IN ('household', 'channel')),
    scope_id      TEXT   REFERENCES channels(id) ON DELETE CASCADE,
    target_key    TEXT   NOT NULL,
    action        TEXT   NOT NULL CHECK (action IN ('keep', 'less', 'never', 'surprise', 'clear')),
    reason        TEXT   NOT NULL DEFAULT '',
    created_at_ns BIGINT NOT NULL CHECK (created_at_ns > 0),
    CHECK ((scope = 'household' AND scope_id IS NULL) OR (scope = 'channel' AND scope_id IS NOT NULL))
);
INSERT INTO discovery_feedback
SELECT id, actor_id, scope, NULLIF(scope_id, ''), target_key, action, reason, created_at_ns
FROM discovery_feedback_old f
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = f.actor_id)
  AND (f.scope = 'household' OR EXISTS (SELECT 1 FROM channels c WHERE c.id = f.scope_id));
DROP TABLE discovery_feedback_old;
CREATE INDEX idx_discovery_feedback_scope_created
    ON discovery_feedback (scope, scope_id, created_at_ns DESC, id DESC);
CREATE INDEX idx_discovery_feedback_actor ON discovery_feedback (actor_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
