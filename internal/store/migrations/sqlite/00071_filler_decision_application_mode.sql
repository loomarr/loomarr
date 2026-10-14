-- +goose Up
-- Every pre-migration decision came from the shadow-only production writer. Keep the default fail-closed
-- for raw inserts while domain writers are required to provide the closed value explicitly.

ALTER TABLE filler_admission_decisions
  ADD COLUMN application_mode TEXT NOT NULL DEFAULT 'shadow'
  CHECK (application_mode IN ('shadow', 'applied'));

-- Forward-only (§16).

-- +goose Down
SELECT 1;
