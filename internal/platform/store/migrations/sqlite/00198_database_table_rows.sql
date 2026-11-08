-- +goose Up
-- Schema to a 10 (#2793, item 5): retention you can watch. The nightly housekeeping pass records
-- every table's row count once a day, so a table that only grows shows as a rising line instead
-- of being found by reading prose. Keyed by day first, so the latest snapshot and the retention
-- purge both read the key.
CREATE TABLE database_table_rows (
    recorded_at INTEGER NOT NULL,
    table_name  TEXT    NOT NULL,
    row_count   INTEGER NOT NULL CHECK (row_count >= 0),
    PRIMARY KEY (recorded_at, table_name)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
