-- +goose Up
CREATE TABLE interactive_operations (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    subject         TEXT NOT NULL,
    status          TEXT NOT NULL,
    percent         INTEGER NOT NULL DEFAULT 0,
    completed       BIGINT NOT NULL DEFAULT 0,
    total           BIGINT NOT NULL DEFAULT 0,
    result_id       TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    started_at      BIGINT NOT NULL,
    completed_at    BIGINT NOT NULL DEFAULT 0,
    updated_at      BIGINT NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
