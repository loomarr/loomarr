-- +goose Up
-- V59 (§10). Postgres mirror of sqlite 00067; the full reasoning lives there.

CREATE TABLE IF NOT EXISTS filler_acquisition_runs (
  id           TEXT PRIMARY KEY,
  trigger      TEXT NOT NULL,
  source_id    TEXT NOT NULL DEFAULT '',
  pull_id      TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'queued',
  requested    INTEGER NOT NULL DEFAULT 0,
  fetched      INTEGER NOT NULL DEFAULT 0,
  skipped      INTEGER NOT NULL DEFAULT 0,
  failed       INTEGER NOT NULL DEFAULT 0,
  empty_count  INTEGER NOT NULL DEFAULT 0,
  error        TEXT NOT NULL DEFAULT '',
  started_at   BIGINT NOT NULL,
  completed_at BIGINT NOT NULL DEFAULT 0,
  updated_at   BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_filler_acquisition_runs_recent
  ON filler_acquisition_runs(started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_acquisition_runs_source
  ON filler_acquisition_runs(source_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_filler_acquisition_runs_pull
  ON filler_acquisition_runs(pull_id, started_at DESC);

ALTER TABLE filler_clip_pipeline
  ADD COLUMN acquisition_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_filler_clip_pipeline_acquisition
  ON filler_clip_pipeline(acquisition_id);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
