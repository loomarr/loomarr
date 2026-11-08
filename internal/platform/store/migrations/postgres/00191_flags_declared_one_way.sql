-- +goose Up
-- Schema to a 10 (#2793): SQLite's flags become BOOLEAN NOT NULL CHECK (col IN (0, 1)) and
-- images.meta gains CHECK (json_valid(meta)) (see the SQLite twin). Postgres already declares
-- them boolean and jsonb; nothing to change.
SELECT 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
