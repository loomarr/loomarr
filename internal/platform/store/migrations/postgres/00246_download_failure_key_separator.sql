-- +goose Up
-- SQLite's 00246 rewrites NUL separators in filler_download_failures.item_key to the unit
-- separator (0x1F). Postgres text never held a NUL, so the table could not have a row to rewrite:
-- this keeps the two migration sequences in step.
SELECT 1;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
