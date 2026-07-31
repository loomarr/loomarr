-- +goose Up
-- §10 compilation splitting (V34): the persisted split proposal — review is not optional and
-- detection runs minutes per file, so the proposal survives a restart and a delayed review.
-- Segments are one JSON document (authored by the detector, edited by the reviewer as a unit,
-- never queried relationally). ONE proposal per compilation clip: re-detection replaces it.
-- Full rationale on the SQLite twin. Forward-only (§16).

CREATE TABLE IF NOT EXISTS filler_split_proposals (
  id            TEXT PRIMARY KEY,
  clip_path     TEXT NOT NULL UNIQUE,
  segments_json TEXT NOT NULL,
  created_at    INTEGER NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
