-- +goose Up
-- V63 (§10). Postgres mirror of sqlite 00070; the full reasoning lives there.

ALTER TABLE filler_admission_actions
  DROP CONSTRAINT filler_admission_actions_kind_check;
ALTER TABLE filler_admission_actions
  ADD CONSTRAINT filler_admission_actions_kind_check
  CHECK (kind IN ('admit', 'reject', 'correct', 'abandon', 'restore', 'reverse'));

-- Forward-only (§16).

-- +goose Down
SELECT 1;
