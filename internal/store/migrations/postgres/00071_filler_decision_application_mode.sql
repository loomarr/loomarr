-- +goose Up
-- Postgres mirror of sqlite 00071; every existing production decision is shadow-only.

ALTER TABLE filler_admission_decisions
  ADD COLUMN application_mode TEXT NOT NULL DEFAULT 'shadow'
  CHECK (application_mode IN ('shadow', 'applied'));

-- Forward-only (§16).

-- +goose Down
SELECT 1;
