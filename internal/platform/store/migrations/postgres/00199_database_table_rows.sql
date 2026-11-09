-- +goose Up
-- Schema to a 10 (#2793, item 5): the SQLite twin explains the table.
CREATE TABLE database_table_rows (
    recorded_at BIGINT NOT NULL,
    table_name  TEXT   NOT NULL,
    row_count   BIGINT NOT NULL CHECK (row_count >= 0),
    PRIMARY KEY (recorded_at, table_name)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
