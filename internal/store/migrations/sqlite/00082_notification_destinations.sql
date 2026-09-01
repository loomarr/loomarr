-- +goose Up
-- Provider credentials live only on the durable destination and are resolved after claim; queued
-- attempts retain only this table's opaque id and a safe label (§11).
CREATE TABLE notification_destinations (
    id                  TEXT PRIMARY KEY,
    means               TEXT NOT NULL CHECK (means IN (
        'email', 'webhook', 'discord', 'ntfy', 'gotify', 'apprise', 'pushover',
        'telegram', 'mattermost', 'matrix', 'web_push', 'mqtt', 'slack'
    )),
    label               TEXT NOT NULL,
    scope               TEXT NOT NULL CHECK (scope IN ('installation', 'person')),
    owner_id            TEXT NOT NULL DEFAULT '',
    audience            TEXT NOT NULL CHECK (audience IN ('person', 'approvers', 'operators')),
    topics_json         TEXT NOT NULL,
    enabled             INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    configuration_json  TEXT NOT NULL DEFAULT '{}',
    credentials_json    TEXT NOT NULL DEFAULT '{}',
    created_at          BIGINT NOT NULL,
    updated_at          BIGINT NOT NULL,
    CHECK (
        (scope = 'installation' AND owner_id = '' AND audience IN ('approvers', 'operators')) OR
        (scope = 'person' AND owner_id <> '' AND audience IN ('person', 'approvers', 'operators'))
    )
);

CREATE INDEX idx_notification_destinations_routing
    ON notification_destinations (enabled, scope, audience, means, id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
