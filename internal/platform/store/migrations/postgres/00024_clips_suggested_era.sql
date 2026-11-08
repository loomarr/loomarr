-- +goose Up
-- §10 era grounding (V34): an AI-proposed era whose year does NOT appear in the clip's text
-- signals is recorded as a SUGGESTION for the operator, never as the `era` tag — full rationale
-- on the SQLite twin. Nothing in pod matching reads this column; confirming it is a PATCH that
-- sets `era`, which clears the suggestion in the same write.
--
-- 0 = no suggestion. Forward-only (§16).
ALTER TABLE clips ADD COLUMN suggested_era INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
