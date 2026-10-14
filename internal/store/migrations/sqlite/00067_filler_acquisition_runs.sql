-- +goose Up
-- V59 (§10): durable filler acquisition runs and clip outcome attribution.
--
-- A transient filler_ingest SSE frame cannot answer what happened after reconnect or restart.
-- Keep downloader facts on one bounded run row, and link pipeline rows to that run so current
-- admitted/review/rejected outcomes remain derivable through the pipeline lifecycle model.

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
  started_at   INTEGER NOT NULL,
  completed_at INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL
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
