-- +goose Up
-- #3855: Loomarr lets go of a clip it fetched and rejected a week ago. Its files are deleted and its
-- pipeline row stays as the record of why; `files_deleted_at` says when the files went, so the row
-- is no longer offered for a retry. 0 means the files are kept.
ALTER TABLE filler_clip_pipeline ADD COLUMN files_deleted_at BIGINT NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
