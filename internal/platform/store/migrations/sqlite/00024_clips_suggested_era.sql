-- +goose Up
-- §10 era grounding (V34): an AI-proposed era whose year does NOT appear in the clip's text
-- signals (filename / sidecar / transcript) is demoted from `era` to a SUGGESTION — measured,
-- the model inferred a decade from tone on 2 of 10 real transcripts, and the validator had no
-- way to tell an inferred year from a read one (plan §6.4). The suggestion is a question for
-- the operator, never a tag: nothing in pod matching reads this column, and confirming it is a
-- PATCH that sets `era` (which clears the suggestion in the same write).
--
-- 0 = no suggestion. NOT NULL DEFAULT 0 keeps the synced-cache story simple (see 00013) —
-- existing clips simply have no suggestion.
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN suggested_era INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
