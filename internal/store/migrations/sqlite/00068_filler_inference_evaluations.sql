-- +goose Up
-- V62 (§10): immutable-after-settlement filler inference attribution and atomic spend reservations.
-- Provider prices drift, so the raw charged decimal and the immutable price snapshot are retained;
-- nanodollars are the integer accounting projection used for bounded sums.

CREATE TABLE IF NOT EXISTS filler_inference_evaluations (
  id                    TEXT PRIMARY KEY,
  cache_key             TEXT NOT NULL,
  clip_hash             TEXT NOT NULL,
  run_id                TEXT NOT NULL DEFAULT '',
  role                  TEXT NOT NULL,
  rung                  TEXT NOT NULL DEFAULT '',
  state                 TEXT NOT NULL,
  requested_provider    TEXT NOT NULL,
  requested_model       TEXT NOT NULL,
  resolved_provider     TEXT NOT NULL DEFAULT '',
  resolved_model        TEXT NOT NULL DEFAULT '',
  upstream_provider     TEXT NOT NULL DEFAULT '',
  modalities_json       TEXT NOT NULL DEFAULT '[]',
  derivative_bytes      INTEGER NOT NULL DEFAULT 0,
  derivative_duration_ms INTEGER NOT NULL DEFAULT 0,
  derivative_pixels     INTEGER NOT NULL DEFAULT 0,
  prompt_tokens         INTEGER NOT NULL DEFAULT 0,
  completion_tokens     INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens      INTEGER NOT NULL DEFAULT 0,
  cached_tokens         INTEGER NOT NULL DEFAULT 0,
  cache_write_tokens    INTEGER NOT NULL DEFAULT 0,
  image_tokens          INTEGER NOT NULL DEFAULT 0,
  audio_tokens          INTEGER NOT NULL DEFAULT 0,
  video_tokens          INTEGER NOT NULL DEFAULT 0,
  charged_amount        TEXT NOT NULL DEFAULT '',
  charged_currency      TEXT NOT NULL DEFAULT '',
  charged_nano_usd      INTEGER NOT NULL DEFAULT 0,
  estimated_nano_usd    INTEGER NOT NULL DEFAULT 0,
  reserved_nano_usd     INTEGER NOT NULL DEFAULT 0,
  price_snapshot        TEXT NOT NULL DEFAULT '',
  latency_ms            INTEGER NOT NULL DEFAULT 0,
  attempts              INTEGER NOT NULL DEFAULT 0,
  generation_id         TEXT NOT NULL DEFAULT '',
  evidence_version      TEXT NOT NULL,
  extractor_version     TEXT NOT NULL,
  prompt_version        TEXT NOT NULL,
  schema_version        TEXT NOT NULL,
  taxonomy_version      TEXT NOT NULL,
  admission_policy_version TEXT NOT NULL,
  role_policy_version   TEXT NOT NULL,
  capability_snapshot   TEXT NOT NULL,
  outcome               TEXT NOT NULL DEFAULT '',
  failure_reason        TEXT NOT NULL DEFAULT '',
  created_at            INTEGER NOT NULL,
  updated_at            INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_filler_inference_clip_created
  ON filler_inference_evaluations(clip_hash, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_inference_cache_key
  ON filler_inference_evaluations(cache_key, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_inference_run_created
  ON filler_inference_evaluations(run_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_filler_inference_day_budget
  ON filler_inference_evaluations(created_at, reserved_nano_usd);

CREATE TABLE IF NOT EXISTS filler_inference_budget_guards (
  scope TEXT PRIMARY KEY
);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
