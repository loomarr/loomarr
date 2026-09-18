-- +goose Up
-- Existing rows have no trustworthy whole-attempt timing. Keep their generation at zero and
-- progress unknown; newly enrolled or explicitly restarted rows populate these fields.
ALTER TABLE filler_clip_pipeline ADD COLUMN preparation_attempt INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_clip_pipeline ADD COLUMN preparation_started_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_clip_pipeline ADD COLUMN preparation_start_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_clip_pipeline ADD COLUMN preparation_progress INTEGER NOT NULL DEFAULT -1;
ALTER TABLE filler_clip_pipeline ADD COLUMN stage_queued_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filler_clip_pipeline ADD COLUMN stage_started_at INTEGER NOT NULL DEFAULT 0;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
