-- +goose Up
-- #749: the channel coverage gap a download was acquired for ("era:1990-1999"), '' when the
-- acquisition was not steered by one. The key a gap-yield read groups by.
ALTER TABLE filler_acquisition_artifacts ADD COLUMN gap TEXT NOT NULL DEFAULT '';

-- Forward-only (§16).

-- +goose Down
SELECT 1;
