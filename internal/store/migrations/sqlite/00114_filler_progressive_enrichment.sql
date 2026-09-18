-- +goose Up
-- Progressive descriptive enrichment is independent from filler readiness. One row records the
-- accepted answer and exact provenance for one clip/axis; no row is the ordinary `missing` state.
CREATE TABLE filler_enrichment_axes (
  clip_hash          TEXT NOT NULL,
  axis               TEXT NOT NULL,
  state              TEXT NOT NULL,
  value_json         TEXT NOT NULL DEFAULT '{}',
  evidence_kind      TEXT NOT NULL,
  evidence_rank      INTEGER NOT NULL,
  evidence_reference TEXT NOT NULL,
  confidence         INTEGER NOT NULL,
  producer           TEXT NOT NULL,
  producer_version   TEXT NOT NULL,
  taxonomy_version   TEXT NOT NULL DEFAULT '',
  observed_at        INTEGER NOT NULL,
  updated_at         INTEGER NOT NULL,
  PRIMARY KEY (clip_hash, axis)
);
CREATE INDEX idx_filler_enrichment_state_axis ON filler_enrichment_axes (state, axis);

-- A completed producer/version pass is separate from the accepted axis projection. A later stronger
-- result may replace the visible axis without making the deterministic pass look undone.
CREATE TABLE filler_enrichment_passes (
  clip_hash        TEXT NOT NULL,
  producer         TEXT NOT NULL,
  producer_version TEXT NOT NULL,
  taxonomy_version TEXT NOT NULL DEFAULT '',
  completed_at     INTEGER NOT NULL,
  PRIMARY KEY (clip_hash, producer, producer_version, taxonomy_version)
);
CREATE INDEX idx_filler_enrichment_passes_completed ON filler_enrichment_passes (producer, producer_version, taxonomy_version, completed_at);

CREATE TABLE filler_enrichment_backfills (
  version      INTEGER PRIMARY KEY,
  completed_at INTEGER NOT NULL
);

-- The original taxonomy seed intentionally no-ops once an installation has any vocabulary. Additive
-- revisions need a separate one-time marker so new shipped terms converge without recreating a term
-- after an operator later removes it.
CREATE TABLE taxonomy_seed_revisions (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
