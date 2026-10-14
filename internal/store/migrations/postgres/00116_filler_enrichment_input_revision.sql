-- +goose Up
-- See the SQLite twin: this revision advances only when a descriptive input changes.
ALTER TABLE clips ADD COLUMN enrichment_revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE filler_enrichment_passes ADD COLUMN input_revision BIGINT NOT NULL DEFAULT 1;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
