-- +goose Up
-- Schema naming (#2323): foreign keys follow the policy in storage.md.
-- A crash report outlives the account that sent it: deleting a user clears `user_id` instead of
-- erasing the report. An airing belongs to its channel: deleting a channel deletes its airings, as
-- it already deletes the channel's feedback. Airings of channels deleted before this migration are
-- pruned first, because nothing could read them.
-- SQLite cannot change a column's foreign key, so both tables are rebuilt transactionally. Neither
-- has children.
DELETE FROM airings WHERE channel_id NOT IN (SELECT id FROM channels);

DROP INDEX idx_airings_channel;
ALTER TABLE airings RENAME TO airings_old;
CREATE TABLE airings (
    channel_id      TEXT   NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    library_item_id TEXT   NOT NULL,           -- the unit that aired (episode or film)
    aired_at        BIGINT NOT NULL,           -- unix seconds the programme started
    recorded_at     BIGINT NOT NULL,           -- unix seconds the row was first written
    key             TEXT   NOT NULL DEFAULT '', -- provision.Key it belongs to (diagnostics)
    PRIMARY KEY (channel_id, library_item_id, aired_at)
);
INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at, key)
SELECT channel_id, library_item_id, aired_at, recorded_at, key FROM airings_old;
DROP TABLE airings_old;
CREATE INDEX idx_airings_channel ON airings (channel_id, recorded_at);

DROP INDEX idx_client_crash_reports_received;
ALTER TABLE client_crash_reports RENAME TO client_crash_reports_old;
CREATE TABLE client_crash_reports (
    id               TEXT PRIMARY KEY,
    received_at_ms   BIGINT NOT NULL CHECK (received_at_ms > 0),
    occurred_at_ms   BIGINT NOT NULL,
    user_id          TEXT REFERENCES users(id) ON DELETE SET NULL,
    device_id        TEXT NOT NULL DEFAULT '',
    device_name      TEXT NOT NULL DEFAULT '',
    kind             TEXT NOT NULL CHECK (kind IN ('js-fatal', 'js-error', 'native')),
    platform         TEXT NOT NULL CHECK (platform IN ('ios', 'android', 'tvos', 'android-tv')),
    app_version      TEXT NOT NULL,
    app_build        TEXT NOT NULL,
    os_version       TEXT NOT NULL,
    device_model     TEXT NOT NULL,
    message          TEXT NOT NULL,
    stack            TEXT NOT NULL DEFAULT '',
    breadcrumbs_json TEXT NOT NULL DEFAULT '[]',
    native_payload   TEXT NOT NULL DEFAULT ''
);
INSERT INTO client_crash_reports (id, received_at_ms, occurred_at_ms, user_id, device_id, device_name,
    kind, platform, app_version, app_build, os_version, device_model, message, stack, breadcrumbs_json,
    native_payload)
SELECT id, received_at_ms, occurred_at_ms, user_id, device_id, device_name,
    kind, platform, app_version, app_build, os_version, device_model, message, stack, breadcrumbs_json,
    native_payload
  FROM client_crash_reports_old;
DROP TABLE client_crash_reports_old;
CREATE INDEX idx_client_crash_reports_received ON client_crash_reports (received_at_ms, id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
