-- +goose Up
-- §10 compilation splitting (V34): the persisted split proposal. Detection runs minutes per
-- file and review is NOT optional (detection quality is a property of the source, measured
-- 69–100% across six real compilations, plan §6.4), so the proposal must survive both the
-- operator taking a day to look at it and a restart in between. Nothing here is a clip: a row
-- becomes catalog rows only through the confirm path, carrying the operator's edits.
--
-- The segments are one JSON document rather than a child table for the same reason channel
-- policy is `policy_json`: the shape is authored by the detector and edited by the reviewer as
-- a unit, never queried relationally, and a per-segment table would buy constraints on data
-- that is definitionally provisional.
--
-- ONE proposal per compilation clip: re-running detection replaces the pending proposal
-- (clip_path UNIQUE), because two competing cut-lists for one file is a review bug, not a
-- choice — the newer detection reflects the same file re-examined.

CREATE TABLE IF NOT EXISTS filler_split_proposals (
  id            TEXT PRIMARY KEY,
  clip_path     TEXT NOT NULL UNIQUE,  -- the compilation's clip identity (path rel. FILLER_DIR)
  segments_json TEXT NOT NULL,         -- []filler.SplitSegment, detector-authored, reviewer-edited
  created_at    INTEGER NOT NULL
);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
