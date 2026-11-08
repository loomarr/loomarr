-- +goose Up
-- The defect floor's record (#2034): one row per clip, the latest measurement and the verdict it
-- earned against the bar for the clip's era. Record-only until enforcement (#2036). A row measured
-- under an older measurement_version, or judged under an older floor_version or a since-corrected
-- era, is measured or judged again. Derived from the bytes, so a re-key drops it.
CREATE TABLE filler_clip_defects (
    clip_hash TEXT PRIMARY KEY,
    measurement_version TEXT NOT NULL,
    floor_version TEXT NOT NULL,
    era INTEGER NOT NULL,
    outcome TEXT NOT NULL,
    readings_json TEXT NOT NULL,
    assessment_json TEXT NOT NULL,
    measured_at INTEGER NOT NULL
);

CREATE INDEX idx_filler_clip_defects_outcome ON filler_clip_defects(outcome);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
