-- +goose Up
-- Schema to a 10 (#2793, item 6): the same indexes as the SQLite twin, which explains why each
-- lookup needed one. Index parity holds both engines to one set.
CREATE INDEX idx_proposal_jobs_kind_updated ON proposal_jobs (kind, updated_at DESC, id DESC);
CREATE INDEX idx_proposal_jobs_channel ON proposal_jobs (channel_id);
CREATE INDEX idx_airings_library_item ON airings (library_item_id);
CREATE INDEX idx_titles_library_id ON titles (library_id);
CREATE INDEX idx_clips_parent ON clips (parent_hash);
CREATE INDEX idx_filler_inferior_copies_survivor ON filler_inferior_copies (survivor_hash);
DROP INDEX idx_filler_acquisition_artifacts_clip;
CREATE INDEX idx_filler_acquisition_artifacts_clip ON filler_acquisition_artifacts (clip_hash);
CREATE INDEX idx_filler_acquisition_artifacts_sidecar ON filler_acquisition_artifacts (sidecar_path);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
