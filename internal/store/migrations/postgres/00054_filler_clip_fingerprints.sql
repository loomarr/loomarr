-- +goose Up
-- V54 (§10). Postgres mirror of sqlite 00054; the ownership reasoning lives there.

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
