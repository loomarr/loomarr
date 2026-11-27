-- +goose Up
-- #4140: a channel's window on air does not move when library facts change.
--
-- `dealt_from` fingerprints what an operator had decided when the accepted cycle was dealt
-- (lineup, policy, break and window settings) and the deal itself. A reconcile that finds the
-- decisions unchanged keeps the window and only heals a vanished item; one that finds them changed
-- re-deals after the programme on air. '' means not yet recorded, and reads as unchanged.
--
-- `playout_anchor_ms` is the milliseconds past `playout_anchor`'s second. A re-deal can start the
-- cycle at the programme on air, which rarely starts on a whole second; without them the walk
-- would move what is on air by up to a second. An older binary ignores the column and reads the
-- anchor truncated, as it always has.
ALTER TABLE channels ADD COLUMN dealt_from TEXT NOT NULL DEFAULT '';
ALTER TABLE channels ADD COLUMN playout_anchor_ms INTEGER NOT NULL DEFAULT 0 CHECK (playout_anchor_ms BETWEEN 0 AND 999);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
