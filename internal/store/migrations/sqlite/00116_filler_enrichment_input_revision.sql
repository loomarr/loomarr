-- +goose Up
-- General catalog updated_at advances on every folder scan. Progressive enrichment needs a
-- narrower wake-up identity or every routine scan would repay completed model work.
ALTER TABLE clips ADD COLUMN enrichment_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE filler_enrichment_passes ADD COLUMN input_revision INTEGER NOT NULL DEFAULT 1;

-- Forward-only (§16).

-- +goose Down
SELECT 1;
