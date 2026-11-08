-- +goose Up
-- Schema to a 10 (#2793): Postgres's integer columns widen to SQLite's 64 bits and images.meta
-- becomes text (see the Postgres twin). SQLite already stores both that way; nothing to change.
SELECT 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
