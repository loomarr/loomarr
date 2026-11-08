-- +goose Up
-- Schema naming (#2323): `titles.available_at` is when the library confirmed the title arrived
-- (Home's New this week), in unix seconds. `available_at` elsewhere means "not before" (a
-- notification attempt's), so this one is named for what it records. The index takes the
-- `idx_<table>_<purpose>` form.
ALTER TABLE titles RENAME COLUMN available_at TO arrived_at;
ALTER INDEX idx_titles_available_at RENAME TO idx_titles_arrived;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
