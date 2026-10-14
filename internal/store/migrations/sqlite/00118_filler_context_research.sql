-- +goose Up
-- Context research is a cited, non-authorizing suggestion and therefore remains separate from the
-- accepted enrichment axes that feed catalog facts and scheduling.
CREATE TABLE filler_context_reports (
  clip_hash        TEXT NOT NULL,
  producer         TEXT NOT NULL,
  producer_version TEXT NOT NULL,
  adapter          TEXT NOT NULL,
  adapter_version  TEXT NOT NULL,
  input_revision   INTEGER NOT NULL,
  report_json      TEXT NOT NULL,
  completed_at     INTEGER NOT NULL,
  PRIMARY KEY (clip_hash, producer, producer_version, adapter, adapter_version, input_revision)
);
CREATE INDEX idx_filler_context_reports_latest ON filler_context_reports (clip_hash, completed_at DESC);

-- Forward-only (§16).

-- +goose Down
SELECT 1;

