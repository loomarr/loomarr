-- +goose Up
-- Crash reports from paired apps (#1816), kept inside Loomarr for admins to read. Who sent one is
-- the server's record of the authenticated request: user_id is NULL for an API-token caller and
-- device_id/device_name are empty for a browser session. A person's reports go with them.
-- Housekeeping keeps 30 days or the newest 1000, whichever is fewer.
CREATE TABLE client_crash_reports (
    id               TEXT PRIMARY KEY,
    received_at      BIGINT NOT NULL CHECK (received_at > 0),
    occurred_at      BIGINT NOT NULL,
    user_id          TEXT REFERENCES users(id) ON DELETE CASCADE,
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

-- Serves both the newest-first admin list and housekeeping's oldest-first purge.
CREATE INDEX idx_client_crash_reports_received ON client_crash_reports (received_at, id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
