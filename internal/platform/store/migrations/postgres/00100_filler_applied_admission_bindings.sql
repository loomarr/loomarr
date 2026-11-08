-- +goose Up
-- Postgres mirror of sqlite 00097; the full reasoning lives there.

ALTER TABLE filler_admission_decisions
  ADD COLUMN screening_evidence_sha256 TEXT NOT NULL DEFAULT ''
  CHECK (
    (application_mode = 'shadow' AND screening_evidence_sha256 = '') OR
    (application_mode = 'applied' AND screening_evidence_sha256 ~ '^[0-9a-f]{64}$')
  );

ALTER TABLE filler_admission_decisions
  ADD COLUMN release_authority_sha256 TEXT NOT NULL DEFAULT ''
  CHECK (
    (application_mode = 'shadow' AND release_authority_sha256 = '') OR
    (application_mode = 'applied' AND release_authority_sha256 ~ '^[0-9a-f]{64}$')
  );

-- Forward-only (§16).

-- +goose Down
SELECT 1;
