-- +goose Up
CREATE TABLE discovery_feedback (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('household', 'channel')),
    scope_id TEXT NOT NULL DEFAULT '',
    target_key TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('keep', 'less', 'never', 'surprise', 'clear')),
    reason TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL CHECK (created_at > 0),
    CHECK ((scope = 'household' AND scope_id = '') OR (scope = 'channel' AND scope_id <> ''))
);
CREATE INDEX discovery_feedback_scope_created
    ON discovery_feedback (scope, scope_id, created_at DESC, id DESC);

-- +goose Down
SELECT 1;
