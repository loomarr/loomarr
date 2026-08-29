-- +goose Up
ALTER TABLE jobs ADD COLUMN failure_trace_json TEXT NOT NULL DEFAULT '';
-- +goose Down
-- forward-only migration
