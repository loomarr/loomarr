-- +goose Up
-- #1069 slice 1: unclassified is persisted held work, never playable catalog state.
-- Repair first so a manually-authored pre-release value cannot prevent the invariant from landing.
UPDATE clips SET held = TRUE, auto_filed = FALSE WHERE kind = 'unclassified' AND held = FALSE;

ALTER TABLE clips
  ADD CONSTRAINT clips_unclassified_held CHECK (kind <> 'unclassified' OR held = TRUE);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
