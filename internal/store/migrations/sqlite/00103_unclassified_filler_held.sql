-- +goose Up
-- #1069 slice 1: unclassified is persisted held work, never playable catalog state.
-- Repair first so a manually-authored pre-release value cannot prevent the invariant from landing.
UPDATE clips SET held = 1, auto_filed = 0 WHERE kind = 'unclassified' AND held = 0;

-- SQLite cannot add a CHECK constraint without rebuilding this heavily referenced table. These
-- two forward-only triggers enforce the same invariant on both entry and every later transition.
-- +goose StatementBegin
CREATE TRIGGER clips_unclassified_held_insert
BEFORE INSERT ON clips
WHEN NEW.kind = 'unclassified' AND NEW.held = 0
BEGIN
  SELECT RAISE(ABORT, 'unclassified filler must remain held');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER clips_unclassified_held_update
BEFORE UPDATE OF kind, held ON clips
WHEN NEW.kind = 'unclassified' AND NEW.held = 0
BEGIN
  SELECT RAISE(ABORT, 'unclassified filler must remain held');
END;
-- +goose StatementEnd

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
