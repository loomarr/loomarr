-- +goose Up
-- Applied V61 decisions bind the exact five-axis aggregate and immutable release authority that
-- terminal admission must replay. Existing and current production rows are shadow, so the empty
-- defaults preserve their truthful non-authorizing state.

ALTER TABLE filler_admission_decisions
  ADD COLUMN screening_evidence_sha256 TEXT NOT NULL DEFAULT ''
  CHECK (
    (application_mode = 'shadow' AND screening_evidence_sha256 = '') OR
    (application_mode = 'applied' AND length(screening_evidence_sha256) = 64
      AND screening_evidence_sha256 NOT GLOB '*[^0-9a-f]*')
  );

ALTER TABLE filler_admission_decisions
  ADD COLUMN release_authority_sha256 TEXT NOT NULL DEFAULT ''
  CHECK (
    (application_mode = 'shadow' AND release_authority_sha256 = '') OR
    (application_mode = 'applied' AND length(release_authority_sha256) = 64
      AND release_authority_sha256 NOT GLOB '*[^0-9a-f]*')
  );

-- Forward-only (§16).

-- +goose Down
SELECT 1;
