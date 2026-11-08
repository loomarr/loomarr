-- +goose Up
-- #749. Postgres mirror of SQLite 00126.
ALTER TABLE filler_acquisition_artifacts ADD COLUMN gap TEXT NOT NULL DEFAULT '';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
