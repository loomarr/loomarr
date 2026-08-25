-- +goose Up
-- V63 (§10): immutable filler-admission decisions and append-only operator actions.
-- Canonical JSON is TEXT in both dialects so round-tripped reason/evidence ordering does not
-- acquire Postgres JSONB's representation semantics.

CREATE TABLE IF NOT EXISTS filler_admission_decisions (
  id                TEXT PRIMARY KEY,
  clip_hash         TEXT NOT NULL,
  evidence_hash     TEXT NOT NULL,
  evidence_version  TEXT NOT NULL,
  schema_version    INTEGER NOT NULL,
  policy_version    TEXT NOT NULL,
  taxonomy_version  TEXT NOT NULL,
  outcome_kind      TEXT NOT NULL,
  verdict           TEXT NOT NULL DEFAULT '',
  hold_code         TEXT NOT NULL DEFAULT '',
  retryable         INTEGER NOT NULL DEFAULT 0,
  result_json       TEXT NOT NULL,
  created_at        INTEGER NOT NULL,
  CHECK (
    (outcome_kind = 'semantic' AND verdict IN ('admit', 'reject', 'review') AND hold_code = '') OR
    (outcome_kind = 'operational' AND verdict = '' AND hold_code <> '')
  )
);

CREATE INDEX IF NOT EXISTS idx_filler_admission_decisions_created
  ON filler_admission_decisions(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_admission_decisions_outcome
  ON filler_admission_decisions(outcome_kind, verdict, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_admission_decisions_clip
  ON filler_admission_decisions(clip_hash, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS filler_admission_decision_inference_refs (
  decision_id   TEXT NOT NULL REFERENCES filler_admission_decisions(id),
  evaluation_id TEXT NOT NULL REFERENCES filler_inference_evaluations(id),
  PRIMARY KEY (decision_id, evaluation_id)
);

CREATE TABLE IF NOT EXISTS filler_admission_actions (
  id                 TEXT PRIMARY KEY,
  decision_id        TEXT NOT NULL REFERENCES filler_admission_decisions(id),
  kind               TEXT NOT NULL,
  actor_id           TEXT NOT NULL,
  reason             TEXT NOT NULL DEFAULT '',
  answer             TEXT NOT NULL DEFAULT '',
  corrected_verdict  TEXT NOT NULL DEFAULT '',
  supersedes_id      TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL,
  CHECK (kind IN ('admit', 'reject', 'correct', 'restore', 'reverse')),
  CHECK (
    (kind = 'correct' AND answer <> '' AND corrected_verdict IN ('admit', 'reject')) OR
    (kind <> 'correct' AND corrected_verdict = '')
  )
);

CREATE INDEX IF NOT EXISTS idx_filler_admission_actions_decision
  ON filler_admission_actions(decision_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_admission_actions_created
  ON filler_admission_actions(created_at DESC, id DESC);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
