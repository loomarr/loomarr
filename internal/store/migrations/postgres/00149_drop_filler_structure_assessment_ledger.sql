-- +goose Up
-- #2147: the whole-video structure-assessment ledger retires. The structure-window pipeline
-- (filler_structure_window_call_ledger) is the only structure journal the server writes; nothing in
-- a binary ever reserved or settled a whole-video request, so the table holds no row the server
-- produced. Its indexes go with it. No backfill: there is nothing to carry.
DROP TABLE IF EXISTS filler_structure_assessment_ledger;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
