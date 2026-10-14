-- +goose Up
ALTER TABLE jobs ADD COLUMN failure_code TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1;
