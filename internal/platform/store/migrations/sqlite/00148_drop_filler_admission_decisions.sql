-- +goose Up
-- #2147: the filler admission decision ledger retires. Nothing wrote a decision (the evaluator is
-- development and certification tooling, not a runtime gate) and the Filler Manage panels that read
-- it are gone, so the four tables held only rows 00107 already emptied: the decisions, their
-- inference references, the operator actions and the diagnostic recovery requests.
--
-- Children first; their indexes go with them. No backfill: there is nothing to carry.
DROP TABLE IF EXISTS filler_diagnostic_recovery_actions;
DROP TABLE IF EXISTS filler_admission_actions;
DROP TABLE IF EXISTS filler_admission_decision_inference_refs;
DROP TABLE IF EXISTS filler_admission_decisions;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
