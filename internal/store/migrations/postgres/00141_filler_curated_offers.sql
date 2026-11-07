-- +goose Up
-- The curated sources an install has been offered (#2035). Loomarr ships a vetted list of archive.org
-- collections and YouTube channels; each upgrade adds the entries this install has not been offered
-- yet, switched off, since enabling a source is the operator's consent to fetch. An offer is kept
-- after the operator deletes the source, so a deleted curated source stays deleted.
CREATE TABLE filler_curated_offers (
    source_id TEXT PRIMARY KEY,
    list_version TEXT NOT NULL,
    offered_at BIGINT NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
