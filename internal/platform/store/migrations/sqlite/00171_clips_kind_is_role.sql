-- +goose Up
-- #2323: a clip's `kind` is its role in a break (commercial, bumper, station_id, …), and the code,
-- the API and the domain language now call it that. A pure rename: the values are unchanged.
-- (No index names the column: 00033 rebuilt clips without the old idx_clips_kind.)
ALTER TABLE clips RENAME COLUMN kind TO role;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
