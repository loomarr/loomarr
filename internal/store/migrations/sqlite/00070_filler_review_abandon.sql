-- +goose Up
-- V63 (§10): an explicit review skip is durable but does not resolve the review.

CREATE TABLE filler_admission_actions_v70 (
  id                 TEXT PRIMARY KEY,
  decision_id        TEXT NOT NULL REFERENCES filler_admission_decisions(id),
  kind               TEXT NOT NULL,
  actor_id           TEXT NOT NULL,
  reason             TEXT NOT NULL DEFAULT '',
  answer             TEXT NOT NULL DEFAULT '',
  corrected_verdict  TEXT NOT NULL DEFAULT '',
  supersedes_id      TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL,
  CHECK (kind IN ('admit', 'reject', 'correct', 'abandon', 'restore', 'reverse')),
  CHECK (
    (kind = 'correct' AND answer <> '' AND corrected_verdict IN ('admit', 'reject')) OR
    (kind <> 'correct' AND corrected_verdict = '')
  )
);

INSERT INTO filler_admission_actions_v70
  (id, decision_id, kind, actor_id, reason, answer, corrected_verdict, supersedes_id, created_at)
SELECT id, decision_id, kind, actor_id, reason, answer, corrected_verdict, supersedes_id, created_at
FROM filler_admission_actions;

DROP TABLE filler_admission_actions;
ALTER TABLE filler_admission_actions_v70 RENAME TO filler_admission_actions;

CREATE INDEX idx_filler_admission_actions_decision
  ON filler_admission_actions(decision_id, created_at DESC, id DESC);
CREATE INDEX idx_filler_admission_actions_created
  ON filler_admission_actions(created_at DESC, id DESC);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
