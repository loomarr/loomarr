-- +goose Up
-- V63 extension. PostgreSQL mirror of SQLite 00104; the full reasoning lives there.

CREATE TABLE filler_diagnostic_recovery_actions (
  id          TEXT PRIMARY KEY,
  decision_id TEXT NOT NULL REFERENCES filler_admission_decisions(id),
  action      TEXT NOT NULL CHECK (action = 'retry'),
  actor_id    TEXT NOT NULL,
  created_at  BIGINT NOT NULL
);

CREATE INDEX idx_filler_diagnostic_recovery_actions_decision
  ON filler_diagnostic_recovery_actions(decision_id, created_at DESC, id DESC);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
