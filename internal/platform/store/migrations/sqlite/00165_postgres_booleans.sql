-- +goose Up
-- Schema naming (#2323): PostgreSQL's INTEGER 0/1 flags become BOOLEAN in its own 00164. SQLite has
-- no boolean type (TRUE and FALSE are 1 and 0), so nothing changes here; this file keeps the two
-- dialects' version numbers in step.
SELECT 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
