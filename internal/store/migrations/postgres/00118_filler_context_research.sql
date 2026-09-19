-- +goose Up
-- See the SQLite twin for the authority boundary.
CREATE TABLE filler_context_reports (
  clip_hash        TEXT NOT NULL,
  producer         TEXT NOT NULL,
  producer_version TEXT NOT NULL,
  adapter          TEXT NOT NULL,
  adapter_version  TEXT NOT NULL,
  input_revision   BIGINT NOT NULL,
  report_json      TEXT NOT NULL,
  completed_at     BIGINT NOT NULL,
  PRIMARY KEY (clip_hash, producer, producer_version, adapter, adapter_version, input_revision)
);
CREATE INDEX idx_filler_context_reports_latest ON filler_context_reports (clip_hash, completed_at DESC);

-- Forward-only (§16).

-- +goose Down
SELECT 1;

