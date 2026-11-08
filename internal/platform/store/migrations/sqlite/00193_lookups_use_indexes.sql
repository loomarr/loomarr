-- +goose Up
-- Schema to a 10 (#2793, item 6): every lookup the store makes is proven to use an index
-- (storetest.CheckQueryPlans, run by both conformance suites). These were full scans:
--   - the suggestion list (ListSuggestionJobs) reads one kind of job newest first;
--   - approval checks a channel's other jobs for a newer approved proposal;
--   - re-keying a library id finds a title's and an airing's rows by it;
--   - re-keying a clip finds its composites (clips.parent_hash), the copies it survived
--     (filler_inferior_copies.survivor_hash) and its manifests (filler_acquisition_artifacts).
--     The manifests' index was partial (clip_hash <> ''), which a plain `clip_hash = ?` cannot use;
--   - a clip's manifest is found by media path OR sidecar path OR clip hash, and SQLite answers an
--     OR through indexes only when every branch has one (the sidecar path had none).
-- Several of these columns become foreign keys, whose child side needs the index anyway so that
-- deleting a parent does not scan.
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
