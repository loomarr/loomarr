-- +goose Up
-- Schema naming (#2323): on PostgreSQL a yes/no column is BOOLEAN, as the older ones already are.
-- These eight were written as INTEGER 0/1, so a reader of the schema could not tell a flag from a
-- count, and SQL compared them to 0 and 1. The store now writes TRUE and FALSE, which SQLite (whose
-- INTEGER flags stay as they are) reads as 1 and 0. Each `IN (0, 1)` CHECK goes with its type, and
-- the one partial index keyed on `active = 1` is rebuilt on the boolean.
ALTER TABLE library_answers ALTER COLUMN ignored DROP DEFAULT;
ALTER TABLE library_answers ALTER COLUMN ignored TYPE BOOLEAN USING ignored <> 0;
ALTER TABLE library_answers ALTER COLUMN ignored SET DEFAULT FALSE;

ALTER TABLE library_candidates ALTER COLUMN name_match DROP DEFAULT;
ALTER TABLE library_candidates ALTER COLUMN name_match TYPE BOOLEAN USING name_match <> 0;
ALTER TABLE library_candidates ALTER COLUMN name_match SET DEFAULT FALSE;
ALTER TABLE library_candidates ALTER COLUMN year_match DROP DEFAULT;
ALTER TABLE library_candidates ALTER COLUMN year_match TYPE BOOLEAN USING year_match <> 0;
ALTER TABLE library_candidates ALTER COLUMN year_match SET DEFAULT FALSE;
ALTER TABLE library_candidates ALTER COLUMN length_match DROP DEFAULT;
ALTER TABLE library_candidates ALTER COLUMN length_match TYPE BOOLEAN USING length_match <> 0;
ALTER TABLE library_candidates ALTER COLUMN length_match SET DEFAULT FALSE;

ALTER TABLE library_items ALTER COLUMN grounded DROP DEFAULT;
ALTER TABLE library_items ALTER COLUMN grounded TYPE BOOLEAN USING grounded <> 0;
ALTER TABLE library_items ALTER COLUMN grounded SET DEFAULT TRUE;
ALTER TABLE library_items ALTER COLUMN needs_noticed DROP DEFAULT;
ALTER TABLE library_items ALTER COLUMN needs_noticed TYPE BOOLEAN USING needs_noticed <> 0;
ALTER TABLE library_items ALTER COLUMN needs_noticed SET DEFAULT FALSE;

ALTER TABLE notification_destinations DROP CONSTRAINT notification_destinations_enabled_check;
ALTER TABLE notification_destinations ALTER COLUMN enabled TYPE BOOLEAN USING enabled <> 0;

DROP INDEX idx_secret_data_keys_one_active;
ALTER TABLE secret_data_keys DROP CONSTRAINT secret_data_keys_active_check;
ALTER TABLE secret_data_keys ALTER COLUMN active TYPE BOOLEAN USING active <> 0;
CREATE UNIQUE INDEX idx_secret_data_keys_one_active ON secret_data_keys (active) WHERE active;

-- A unix-seconds time in a 32-bit INTEGER runs out in 2038; every other time column is BIGINT.
ALTER TABLE filler_split_proposals ALTER COLUMN created_at TYPE BIGINT;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
