-- +goose Up
-- V54 (§10): persisted derived fingerprints for compilation de-duplication.
--
-- A sibling of `clips`, not a column on it: `clips` is a rebuildable folder cache, while this
-- table preserves completed ffmpeg work across retries and restarts. There is deliberately no
-- foreign key for the same reason as `filler_clip_pipeline`; catalog pruning removes orphans.
-- The content hash invalidates changed bytes and the algorithm key invalidates detector changes.

CREATE TABLE IF NOT EXISTS filler_clip_fingerprints (
  clip_hash   TEXT NOT NULL,
  algorithm   TEXT NOT NULL,
  frames_json TEXT NOT NULL,
  PRIMARY KEY (clip_hash, algorithm)
);

CREATE INDEX IF NOT EXISTS idx_filler_clip_fingerprints_algorithm
  ON filler_clip_fingerprints(algorithm);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
