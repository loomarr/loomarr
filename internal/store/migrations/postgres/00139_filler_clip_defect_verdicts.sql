-- +goose Up
-- The defect floor's judge (#2034): a vision model's ruling on a borderline assessment, and the
-- clip's final verdict. Rows written before this carry an empty verdict; the hourly floor job
-- decides it on its next pass, from the stored readings.
ALTER TABLE filler_clip_defects ADD COLUMN judgement_json TEXT NOT NULL DEFAULT '';
-- Asks since the last ruling that brought none back; a clip the judge keeps failing on waits
-- behind the rest.
ALTER TABLE filler_clip_defects ADD COLUMN judge_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_clip_defects ADD COLUMN verdict TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_clip_defects ADD COLUMN verdict_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_filler_clip_defects_verdict ON filler_clip_defects(verdict);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
